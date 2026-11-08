package av

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// Layer is a simulcast layer a receiver wants from one sender.
type Layer string

const (
	LayerLow  Layer = "q"
	LayerMid  Layer = "h"
	LayerHigh Layer = "f"
	LayerOff  Layer = "off" // no video from this sender; audio continues
)

// layerOrder lists the real layers from the smallest to the biggest.
var layerOrder = [...]Layer{LayerLow, LayerMid, LayerHigh}

func layerBit(l Layer) uint8 {
	for i, o := range layerOrder {
		if o == l {
			return 1 << i
		}
	}
	return 0
}

func validLayer(l Layer) bool { return l == LayerOff || layerBit(l) != 0 }

// effectiveLayer is the layer to forward for a wanted one, given the bit set of
// layers the sender publishes: the wanted layer, else the nearest smaller
// published one, else the smallest published one. "" when nothing is published.
func effectiveLayer(want Layer, avail uint8) Layer {
	if want == LayerOff {
		return LayerOff
	}
	idx := 0
	for i, o := range layerOrder {
		if o == want {
			idx = i
		}
	}
	for i := idx; i >= 0; i-- {
		if avail&(1<<i) != 0 {
			return layerOrder[i]
		}
	}
	for i, o := range layerOrder {
		if avail&(1<<i) != 0 {
			return o
		}
	}
	return ""
}

// isVP8Keyframe reports whether p starts a VP8 key frame: start of partition 0
// (RFC 7741 payload descriptor) and the P bit of the frame tag cleared.
func isVP8Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	b0 := payload[0]
	start := b0&0x10 != 0
	pid := b0 & 0x07
	if !start || pid != 0 {
		return false
	}
	i := 1
	if b0&0x80 != 0 { // X: extension byte
		if len(payload) <= i {
			return false
		}
		ext := payload[i]
		i++
		if ext&0x80 != 0 { // I: PictureID
			if len(payload) <= i {
				return false
			}
			if payload[i]&0x80 != 0 {
				i++
			}
			i++
		}
		if ext&0x40 != 0 { // L: TL0PICIDX
			i++
		}
		if ext&0x30 != 0 { // T or K: TID/KEYIDX byte
			i++
		}
	}
	if len(payload) <= i {
		return false
	}
	return payload[i]&0x01 == 0
}

// rtpWriter is what a sub writes packets to; *webrtc.TrackLocalStaticRTP fits.
type rtpWriter interface {
	WriteRTP(*rtp.Packet) error
}

// sub is one sender->receiver subscription: the outgoing tracks on the
// receiver's connection plus the state needed to switch simulcast layers
// without breaking the receiver's sequence numbers and timestamps.
type sub struct {
	from, to *Peer
	vtr, atr *webrtc.RTPTransceiver

	vw, aw          rtpWriter
	blocked         *atomic.Bool // the sender's audio block flag
	requestKeyframe func(rid Layer)
	countOut        func(n int)

	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	avail   uint8 // bit set of layers the sender publishes
	cur     Layer // layer currently forwarded ("" until the first key frame)
	want    Layer // layer requested by the receiver
	seqOff  uint16
	tsOff   uint32
	lastSeq uint16
	lastTS  uint32
	lastAt  time.Time
}

func (s *sub) stop() { s.once.Do(func() { close(s.done) }) }

// target is the layer to forward now. Called with s.mu held.
func (s *sub) target() Layer { return effectiveLayer(s.want, s.avail) }

func (s *sub) setWant(l Layer) {
	s.mu.Lock()
	s.want = l
	t := s.target()
	if t == LayerOff {
		s.cur = ""
	}
	s.mu.Unlock()
	if t != LayerOff && t != "" {
		s.requestKeyframe(t)
	}
}

// setAvail records the layers the sender publishes.
func (s *sub) setAvail(mask uint8) {
	s.mu.Lock()
	s.avail = mask
	t := s.target()
	pending := s.cur != t
	s.mu.Unlock()
	if pending && t != LayerOff && t != "" {
		s.requestKeyframe(t)
	}
}

// keyframeLayer is the layer a receiver's PLI or FIR is meant for.
func (s *sub) keyframeLayer() Layer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != "" {
		return s.cur
	}
	return s.target()
}

// pendingLoop keeps asking the sender for a key frame of the wanted layer until
// the switch happened (the first PLI can be lost or arrive before the layer
// has started). It asks for nothing while the video is off.
func (s *sub) pendingLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.mu.Lock()
			want := s.target()
			pending := want != s.cur && want != LayerOff && want != ""
			s.mu.Unlock()
			if pending {
				s.requestKeyframe(want)
			}
		}
	}
}

// forwardVideo is called for every RTP packet of layer rid of the sender.
func (s *sub) forwardVideo(rid Layer, p *rtp.Packet) {
	s.mu.Lock()
	t := s.target()
	if t == LayerOff {
		s.cur = ""
		s.mu.Unlock()
		return
	}
	if rid != s.cur {
		if rid != t || !isVP8Keyframe(p.Payload) {
			s.mu.Unlock()
			return
		}
		if !s.lastAt.IsZero() {
			s.seqOff = s.lastSeq + 1 - p.SequenceNumber
			elapsed := uint32(time.Since(s.lastAt).Milliseconds()) * 90 //nolint:gosec // RTP timestamps wrap modulo 2^32
			if elapsed == 0 {
				elapsed = 3000
			}
			s.tsOff = s.lastTS + elapsed - p.Timestamp
		}
		s.cur = rid
	}
	out := *p
	out.Extension = false
	out.Extensions = nil
	out.SequenceNumber = p.SequenceNumber + s.seqOff
	out.Timestamp = p.Timestamp + s.tsOff
	s.lastSeq = out.SequenceNumber
	s.lastTS = out.Timestamp
	s.lastAt = time.Now()
	s.mu.Unlock()

	if err := s.vw.WriteRTP(&out); err == nil && s.countOut != nil {
		s.countOut(len(p.Payload) + 12)
	}
}

func (s *sub) forwardAudio(p *rtp.Packet) {
	if s.blocked.Load() {
		return
	}
	out := *p
	out.Extension = false
	out.Extensions = nil
	if err := s.aw.WriteRTP(&out); err == nil && s.countOut != nil {
		s.countOut(len(p.Payload) + 12)
	}
}

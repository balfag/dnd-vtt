package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// Simulcast layer ids, as sent by the browser in sendEncodings.
const (
	layerLow  = "q"
	layerMid  = "h"
	layerHigh = "f"
)

func validLayer(l string) bool { return l == layerLow || l == layerMid || l == layerHigh }

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

// sub is one sender->receiver subscription: the outgoing tracks on the
// receiver's connection plus the state needed to switch simulcast layers
// without breaking the receiver's sequence numbers and timestamps.
type sub struct {
	from, to *peer
	vtrack   *webrtc.TrackLocalStaticRTP
	atrack   *webrtc.TrackLocalStaticRTP
	vsender  *webrtc.RTPSender
	asender  *webrtc.RTPSender
	done     chan struct{}
	once     sync.Once

	mu      sync.Mutex
	cur     string // layer currently forwarded ("" until the first key frame)
	want    string // layer requested by the receiver
	seqOff  uint16
	tsOff   uint32
	lastSeq uint16
	lastTS  uint32
	lastAt  time.Time

	switches atomic.Int64
}

func (s *sub) setWant(l string) {
	s.mu.Lock()
	s.want = l
	s.mu.Unlock()
	s.from.requestKeyframe(l)
}

func (s *sub) stop() { s.once.Do(func() { close(s.done) }) }

// pendingLoop keeps asking the sender for a key frame of the wanted layer until
// the switch happened (the first PLI can be lost or arrive before the layer
// has started).
func (s *sub) pendingLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.mu.Lock()
			pending := s.cur != s.want
			want := s.want
			s.mu.Unlock()
			if pending {
				s.from.requestKeyframe(want)
			}
		}
	}
}

// forwardVideo is called for every RTP packet of layer rid of the sender.
func (s *sub) forwardVideo(rid string, p *rtp.Packet, rewrite bool) {
	s.mu.Lock()
	switched := false
	if rid != s.cur {
		if rid != s.want || !isVP8Keyframe(p.Payload) {
			s.mu.Unlock()
			return
		}
		if rewrite && s.cur != "" {
			s.seqOff = s.lastSeq + 1 - p.SequenceNumber
			elapsed := uint32(time.Since(s.lastAt).Milliseconds()) * 90
			if elapsed == 0 {
				elapsed = 3000
			}
			s.tsOff = s.lastTS + elapsed - p.Timestamp
		}
		s.cur = rid
		switched = true
		s.switches.Add(1)
	}
	out := *p
	out.Extension = false
	out.Extensions = nil
	if rewrite {
		out.SequenceNumber = p.SequenceNumber + s.seqOff
		out.Timestamp = p.Timestamp + s.tsOff
	}
	s.lastSeq = out.SequenceNumber
	s.lastTS = out.Timestamp
	s.lastAt = time.Now()
	s.mu.Unlock()

	if switched {
		go s.to.send(&msg{Type: "layer-switched", Peer: s.from.id, Layer: rid})
	}
	if err := s.vtrack.WriteRTP(&out); err == nil {
		s.from.room.bytesOut.Add(int64(len(p.Payload) + 12))
	}
}

func (s *sub) forwardAudio(p *rtp.Packet) {
	out := *p
	out.Extension = false
	out.Extensions = nil
	if err := s.atrack.WriteRTP(&out); err == nil {
		s.from.room.bytesOut.Add(int64(len(p.Payload) + 12))
	}
}

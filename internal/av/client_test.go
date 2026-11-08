package av

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// recvPacket is what the test client keeps of every incoming RTP packet.
type recvPacket struct {
	seq   uint16
	layer byte // third payload byte: which simulcast layer the packet came from
}

// recvTrack is one incoming track of a test client.
type recvTrack struct {
	streamID, id string

	mu    sync.Mutex
	pkts  []recvPacket
	ended chan struct{}
}

func (r *recvTrack) packets() []recvPacket {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recvPacket(nil), r.pkts...)
}

func (r *recvTrack) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pkts)
}

func (r *recvTrack) hasLayer(l byte) bool {
	for _, p := range r.packets() {
		if p.layer == l {
			return true
		}
	}
	return false
}

func (r *recvTrack) isEnded() bool {
	select {
	case <-r.ended:
		return true
	default:
		return false
	}
}

// testClient is a pion client that publishes audio and (simulcast) video the
// way a browser would and records everything the SFU sends back.
type testClient struct {
	t    *testing.T
	id   string
	peer *Peer
	pc   *webrtc.PeerConnection

	failed atomic.Bool

	signals  chan Signal
	answered chan struct{}
	stop     chan struct{}
	wg       sync.WaitGroup

	mu        sync.Mutex
	remoteSet bool
	pendingC  []webrtc.ICECandidateInit
	tracks    map[string]*recvTrack // "<streamID>/<track id>"
	closeOnce sync.Once
}

var simulcastLayers = []string{"q", "h", "f"}

func newTestAPI(t *testing.T) *webrtc.API {
	t.Helper()
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		t.Fatal(err)
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		t.Fatal(err)
	}
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se))
}

// newTestClient joins the room and starts the client; simulcast=false sends a
// single video layer without rid.
func newTestClient(t *testing.T, room *Room, id string, simulcast bool) *testClient {
	t.Helper()
	c := &testClient{
		t: t, id: id,
		signals:  make(chan Signal, 1024),
		answered: make(chan struct{}),
		stop:     make(chan struct{}),
		tracks:   map[string]*recvTrack{},
	}
	peer, err := room.Join(id, func(s Signal) { c.signals <- s }, func() { c.failed.Store(true) })
	if err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
	c.peer = peer
	c.setup(simulcast)
	return c
}

// setup builds the client's connection, tracks and goroutines.
func (c *testClient) setup(simulcast bool) {
	t := c.t
	pc, err := newTestAPI(t).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	c.pc = pc

	pc.OnICECandidate(func(ci *webrtc.ICECandidate) {
		if ci == nil {
			return
		}
		init := ci.ToJSON()
		c.mu.Lock()
		if !c.remoteSet {
			c.pendingC = append(c.pendingC, init)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		_ = c.peer.AddICE(init)
	})
	pc.OnTrack(c.onTrack)

	audio, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "client-"+c.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pc.AddTrack(audio); err != nil {
		t.Fatal(err)
	}
	vp8 := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	var video []*webrtc.TrackLocalStaticRTP
	var sender *webrtc.RTPSender
	var layerBytes []byte
	if simulcast {
		for i, rid := range simulcastLayers {
			tr, err := webrtc.NewTrackLocalStaticRTP(vp8, "video", "client-"+c.id, webrtc.WithRTPStreamID(rid))
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				if sender, err = pc.AddTrack(tr); err != nil {
					t.Fatal(err)
				}
			} else if err := sender.AddEncoding(tr); err != nil {
				t.Fatal(err)
			}
			video = append(video, tr)
			layerBytes = append(layerBytes, rid[0])
		}
	} else {
		tr, err := webrtc.NewTrackLocalStaticRTP(vp8, "video", "client-"+c.id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pc.AddTrack(tr); err != nil {
			t.Fatal(err)
		}
		video = append(video, tr)
		layerBytes = append(layerBytes, 'h')
	}

	c.wg.Add(2)
	go c.worker()
	go c.writer(audio, video, layerBytes, sender)
}

// connect runs the first offer/answer exchange.
func (c *testClient) connect() {
	t := c.t
	t.Helper()
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := c.peer.HandleOffer(offer.SDP)
	if err != nil {
		t.Fatalf("%s: HandleOffer: %v", c.id, err)
	}
	if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.remoteSet = true
	pending := c.pendingC
	c.pendingC = nil
	c.mu.Unlock()
	close(c.answered)
	for _, ci := range pending {
		if err := c.peer.AddICE(ci); err != nil {
			t.Fatal(err)
		}
	}
}

// worker handles the SFU's signals strictly in order, after the first answer.
func (c *testClient) worker() {
	defer c.wg.Done()
	select {
	case <-c.answered:
	case <-c.stop:
		return
	}
	for {
		select {
		case <-c.stop:
			return
		case s := <-c.signals:
			switch s.Type {
			case "offer":
				if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: s.SDP}); err != nil {
					c.t.Errorf("%s: set sfu offer: %v", c.id, err)
					continue
				}
				answer, err := c.pc.CreateAnswer(nil)
				if err == nil {
					err = c.pc.SetLocalDescription(answer)
				}
				if err != nil {
					c.t.Errorf("%s: answer: %v", c.id, err)
					continue
				}
				if err := c.peer.HandleAnswer(answer.SDP); err != nil && !c.isStopped() {
					c.t.Errorf("%s: HandleAnswer: %v", c.id, err)
				}
			case "ice":
				if s.Candidate != nil {
					_ = c.pc.AddICECandidate(*s.Candidate)
				}
			}
		}
	}
}

func (c *testClient) isStopped() bool {
	select {
	case <-c.stop:
		return true
	default:
		return false
	}
}

// writer sends one packet per layer (and one audio packet) every 20 ms. Every
// 15th video packet is a key frame.
func (c *testClient) writer(audio *webrtc.TrackLocalStaticRTP, video []*webrtc.TrackLocalStaticRTP, layerBytes []byte, sender *webrtc.RTPSender) {
	defer c.wg.Done()
	select {
	case <-c.answered:
	case <-c.stop:
		return
	}
	// pion does not add the simulcast header extensions itself; the SFU finds
	// the layer of a packet by them.
	var midID, ridID uint8
	var mid string
	if sender != nil && len(video) > 1 {
		for _, e := range sender.GetParameters().HeaderExtensions {
			switch e.URI {
			case "urn:ietf:params:rtp-hdrext:sdes:mid":
				midID = uint8(e.ID) //nolint:gosec // extension ids are 1..14
			case "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id":
				ridID = uint8(e.ID) //nolint:gosec // extension ids are 1..14
			}
		}
		for _, tr := range c.pc.GetTransceivers() {
			if tr.Sender() == sender {
				mid = tr.Mid()
			}
		}
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	var n uint16
	for {
		select {
		case <-c.stop:
			return
		case <-tick.C:
		}
		_ = audio.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: 100 + n, Timestamp: uint32(n) * 960},
			Payload: []byte{0xF8, 0xFF, 0xFE},
		})
		for i, tr := range video {
			kf := byte(0x01)
			if n%15 == 0 {
				kf = 0x00
			}
			h := rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: uint16(1000*(i+1)) + n, Timestamp: uint32(n) * 1800}
			if ridID != 0 {
				_ = h.SetExtension(midID, []byte(mid))
				_ = h.SetExtension(ridID, []byte{layerBytes[i]})
			}
			_ = tr.WriteRTP(&rtp.Packet{Header: h, Payload: []byte{0x10, kf, layerBytes[i]}})
		}
		n++
	}
}

func (c *testClient) onTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	rt := &recvTrack{streamID: track.StreamID(), id: track.ID(), ended: make(chan struct{})}
	c.mu.Lock()
	c.tracks[rt.streamID+"/"+rt.id] = rt
	c.mu.Unlock()
	defer close(rt.ended)
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		rp := recvPacket{seq: pkt.SequenceNumber}
		if len(pkt.Payload) >= 3 {
			rp.layer = pkt.Payload[2]
		}
		rt.mu.Lock()
		rt.pkts = append(rt.pkts, rp)
		rt.mu.Unlock()
	}
}

// track returns the incoming track "<stream>/<id>" or nil.
func (c *testClient) track(stream, id string) *recvTrack {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tracks[stream+"/"+id]
}

// close stops the client's goroutines and its connection.
func (c *testClient) close() {
	c.closeOnce.Do(func() {
		close(c.stop)
		_ = c.pc.Close()
		c.wg.Wait()
	})
}

// waitFor polls cond until it holds or the timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package av

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pion/rtp"
)

type fakeWriter struct {
	mu   sync.Mutex
	pkts []rtp.Packet
}

func (w *fakeWriter) WriteRTP(p *rtp.Packet) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pkts = append(w.pkts, *p)
	return nil
}

func (w *fakeWriter) all() []rtp.Packet {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]rtp.Packet(nil), w.pkts...)
}

type keyframeLog struct {
	mu   sync.Mutex
	reqs []Layer
}

func (k *keyframeLog) add(l Layer) {
	k.mu.Lock()
	k.reqs = append(k.reqs, l)
	k.mu.Unlock()
}

func newTestSub(avail uint8, want Layer) (*sub, *fakeWriter, *fakeWriter, *keyframeLog) {
	vw, aw, kl := &fakeWriter{}, &fakeWriter{}, &keyframeLog{}
	s := &sub{
		vw: vw, aw: aw, blocked: &atomic.Bool{},
		requestKeyframe: kl.add, avail: avail, want: want,
		done: make(chan struct{}),
	}
	return s, vw, aw, kl
}

const allLayers = 0b111

func vp8(key bool) []byte {
	if key {
		return []byte{0x10, 0x00, 0x01}
	}
	return []byte{0x10, 0x01, 0x01}
}

func pkt(seq uint16, ts uint32, key bool) *rtp.Packet {
	return &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: ts}, Payload: vp8(key)}
}

func TestIsVP8Keyframe(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{"key frame, no extension", []byte{0x10, 0x00}, true},
		{"key frame, extension with picture id", []byte{0x90, 0x80, 0x05, 0x00}, true},
		{"key frame, 15-bit picture id", []byte{0x90, 0x80, 0x85, 0x01, 0x00}, true},
		{"key frame, all extension fields", []byte{0x90, 0xF0, 0x05, 0x01, 0x02, 0x00}, true},
		{"delta frame", []byte{0x10, 0x01}, false},
		{"not the start of a partition", []byte{0x00, 0x00}, false},
		{"partition id is not 0", []byte{0x11, 0x00}, false},
		{"empty", nil, false},
		{"only the descriptor", []byte{0x10}, false},
		{"extension byte missing", []byte{0x90}, false},
		{"picture id missing", []byte{0x90, 0x80}, false},
	}
	for _, c := range cases {
		if got := isVP8Keyframe(c.payload); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEffectiveLayer(t *testing.T) {
	cases := []struct {
		want  Layer
		avail uint8
		out   Layer
	}{
		{LayerHigh, allLayers, LayerHigh},
		{LayerHigh, 0b011, LayerMid},
		{LayerMid, 0b101, LayerLow},
		{LayerLow, 0b110, LayerMid},
		{LayerLow, 0b100, LayerHigh},
		{LayerMid, 0, ""},
		{LayerOff, allLayers, LayerOff},
	}
	for _, c := range cases {
		if got := effectiveLayer(c.want, c.avail); got != c.out {
			t.Errorf("effectiveLayer(%q, %03b) = %q, want %q", c.want, c.avail, got, c.out)
		}
	}
}

func TestSwitchWaitsForKeyframeAndKeepsSequence(t *testing.T) {
	s, vw, _, _ := newTestSub(allLayers, LayerLow)
	s.forwardVideo(LayerLow, pkt(100, 1000, true))
	s.forwardVideo(LayerLow, pkt(101, 4000, false))
	s.setWant(LayerHigh)

	// Layer f is not forwarded until its key frame.
	s.forwardVideo(LayerHigh, pkt(900, 50000, false))
	s.forwardVideo(LayerLow, pkt(102, 7000, false))
	if n := len(vw.all()); n != 3 {
		t.Fatalf("written %d packets before the key frame, want 3", n)
	}
	s.forwardVideo(LayerHigh, pkt(901, 53000, true))
	s.forwardVideo(LayerLow, pkt(103, 10000, false)) // old layer is dropped now
	s.forwardVideo(LayerHigh, pkt(902, 56000, false))

	got := vw.all()
	if len(got) != 5 {
		t.Fatalf("written %d packets, want 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].SequenceNumber != got[i-1].SequenceNumber+1 {
			t.Errorf("packet %d: seq %d after %d", i, got[i].SequenceNumber, got[i-1].SequenceNumber)
		}
		if int32(got[i].Timestamp-got[i-1].Timestamp) <= 0 { //nolint:gosec // wrap-safe timestamp comparison
			t.Errorf("packet %d: timestamp %d does not grow after %d", i, got[i].Timestamp, got[i-1].Timestamp)
		}
	}
	if got[3].SequenceNumber != 103 {
		t.Errorf("seq after the switch = %d, want 103", got[3].SequenceNumber)
	}
}

func TestOffAndBack(t *testing.T) {
	s, vw, _, kl := newTestSub(allLayers, LayerLow)
	s.forwardVideo(LayerLow, pkt(10, 1000, true))
	s.forwardVideo(LayerLow, pkt(11, 4000, false))

	s.setWant(LayerOff)
	s.forwardVideo(LayerLow, pkt(12, 7000, false))
	s.forwardVideo(LayerLow, pkt(13, 10000, true))
	if n := len(vw.all()); n != 2 {
		t.Fatalf("written %d packets while off, want 2", n)
	}
	kl.mu.Lock()
	for _, l := range kl.reqs {
		if l == LayerOff {
			t.Error("key frame requested for the off layer")
		}
	}
	kl.mu.Unlock()

	s.setWant(LayerLow)
	s.forwardVideo(LayerLow, pkt(14, 13000, false)) // waits for a key frame
	if n := len(vw.all()); n != 2 {
		t.Fatalf("written %d packets before the key frame, want 2", n)
	}
	s.forwardVideo(LayerLow, pkt(15, 16000, true))
	s.forwardVideo(LayerLow, pkt(16, 19000, false))
	got := vw.all()
	if len(got) != 4 {
		t.Fatalf("written %d packets, want 4", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].SequenceNumber != got[i-1].SequenceNumber+1 {
			t.Errorf("packet %d: seq %d after %d", i, got[i].SequenceNumber, got[i-1].SequenceNumber)
		}
	}
}

func TestUnavailableLayerFallsBack(t *testing.T) {
	s, vw, _, _ := newTestSub(0b011, LayerHigh) // q and h published
	s.forwardVideo(LayerLow, pkt(1, 0, true))
	s.forwardVideo(LayerMid, pkt(2, 0, true))
	s.forwardVideo(LayerMid, pkt(3, 3000, false))
	got := vw.all()
	if len(got) != 2 {
		t.Fatalf("written %d packets, want 2 (layer h only)", len(got))
	}
}

func TestForwardAudioBlock(t *testing.T) {
	s, _, aw, _ := newTestSub(allLayers, LayerLow)
	p := &rtp.Packet{Payload: []byte{1, 2, 3}}
	s.forwardAudio(p)
	s.blocked.Store(true)
	s.forwardAudio(p)
	s.forwardAudio(p)
	if n := len(aw.all()); n != 1 {
		t.Fatalf("written %d packets, want 1 while blocked", n)
	}
	s.blocked.Store(false)
	s.forwardAudio(p)
	if n := len(aw.all()); n != 2 {
		t.Fatalf("written %d packets after unblock, want 2", n)
	}
}

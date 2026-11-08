package av

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime"
	"testing"
	"time"
)

// Integration tests share a goroutine count (see TestNoGoroutineLeaks), so none
// of them is parallel.

const waitLong = 10 * time.Second

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(Config{MediaPort: 0}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

// join makes a connected client.
func join(t *testing.T, room *Room, id string, simulcast bool) *testClient {
	t.Helper()
	c := newTestClient(t, room, id, simulcast)
	t.Cleanup(c.close)
	c.connect()
	return c
}

// waitTrack waits for the incoming track stream/id with at least n packets.
func waitTrack(t *testing.T, c *testClient, stream, id string, n int) *recvTrack {
	t.Helper()
	var tr *recvTrack
	waitFor(t, waitLong, fmt.Sprintf("%s to receive %s/%s", c.id, stream, id), func() bool {
		tr = c.track(stream, id)
		return tr != nil && tr.count() >= n
	})
	return tr
}

func checkConsecutive(t *testing.T, pkts []recvPacket) {
	t.Helper()
	for i := 1; i < len(pkts); i++ {
		if pkts[i].seq != pkts[i-1].seq+1 {
			t.Fatalf("packet %d: seq %d after %d", i, pkts[i].seq, pkts[i-1].seq)
		}
	}
}

func TestTwoPeersSeeEachOther(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	a := join(t, room, "A", true)
	b := join(t, room, "B", true)

	for _, pair := range [][2]*testClient{{a, b}, {b, a}} {
		rx, from := pair[0], pair[1]
		waitTrack(t, rx, from.id, "audio", 5)
		video := waitTrack(t, rx, from.id, "video", 5)
		for _, p := range video.packets() {
			if p.layer != 'q' {
				t.Fatalf("%s got layer %q from %s, want the default q", rx.id, p.layer, from.id)
			}
		}
	}
	if room.Len() != 2 {
		t.Fatalf("Len = %d, want 2", room.Len())
	}
	waitFor(t, waitLong, "traffic counters", func() bool {
		in, out := room.Traffic()
		return in > 0 && out > 0
	})
}

func TestSwitchLayerKeepsSequence(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	a := join(t, room, "A", true)
	b := join(t, room, "B", true)
	video := waitTrack(t, b, "A", "video", 5)

	b.peer.SetView(map[string]Layer{"A": LayerHigh})
	waitFor(t, waitLong, "layer f", func() bool { return video.hasLayer('f') })
	n := video.count()
	waitFor(t, waitLong, "more f packets", func() bool { return video.count() > n+5 })
	checkConsecutive(t, video.packets())
	_ = a
}

func TestViewOffAndBack(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	join(t, room, "A", true)
	b := join(t, room, "B", true)
	video := waitTrack(t, b, "A", "video", 5)
	audio := waitTrack(t, b, "A", "audio", 5)

	b.peer.SetView(map[string]Layer{"A": LayerOff})
	time.Sleep(200 * time.Millisecond) // packets already on the wire
	v0, a0 := video.count(), audio.count()
	time.Sleep(time.Second)
	if v1 := video.count(); v1 != v0 {
		t.Fatalf("%d video packets arrived while off", v1-v0)
	}
	if audio.count() <= a0 {
		t.Fatal("audio stopped while the video is off")
	}

	b.peer.SetView(map[string]Layer{"A": LayerLow})
	waitFor(t, waitLong, "video after off", func() bool { return video.count() > v0+5 })
	checkConsecutive(t, video.packets())
}

func TestAudioBlock(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	a := join(t, room, "A", true)
	b := join(t, room, "B", true)
	video := waitTrack(t, b, "A", "video", 5)
	audio := waitTrack(t, b, "A", "audio", 5)

	a.peer.SetAudioBlocked(true)
	time.Sleep(200 * time.Millisecond)
	a0, v0 := audio.count(), video.count()
	time.Sleep(time.Second)
	if a1 := audio.count(); a1 != a0 {
		t.Fatalf("%d audio packets arrived while blocked", a1-a0)
	}
	if video.count() <= v0 {
		t.Fatal("video stopped while the audio is blocked")
	}

	a.peer.SetAudioBlocked(false)
	waitFor(t, waitLong, "audio after unblock", func() bool { return audio.count() > a0+5 })
}

func TestThirdPeerJoinsAndLeaves(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	a := join(t, room, "A", true)
	b := join(t, room, "B", true)
	waitTrack(t, a, "B", "video", 1)

	c := join(t, room, "C", true)
	var ended []*recvTrack
	for _, rx := range []*testClient{a, b} {
		waitTrack(t, rx, "C", "audio", 5)
		ended = append(ended, waitTrack(t, rx, "C", "video", 5), rx.track("C", "audio"))
	}
	waitTrack(t, c, "A", "video", 5)
	waitTrack(t, c, "B", "video", 5)
	if room.Len() != 3 {
		t.Fatalf("Len = %d, want 3", room.Len())
	}

	room.Leave("C")
	if room.Len() != 2 {
		t.Fatalf("Len after Leave = %d, want 2", room.Len())
	}
	for _, tr := range ended {
		waitFor(t, waitLong, "C's track to end for "+tr.id, tr.isEnded)
	}
	// The others still get each other's media.
	n := a.track("B", "video").count()
	waitFor(t, waitLong, "A still receiving B", func() bool { return a.track("B", "video").count() > n+5 })

	time.Sleep(300 * time.Millisecond)
	if c.failed.Load() {
		t.Fatal("onFailed called for a peer that left through Leave")
	}
	room.Leave("nobody") // no-op
}

func TestNoGoroutineLeaks(t *testing.T) {
	before := runtime.NumGoroutine()
	e, err := NewEngine(Config{MediaPort: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	room := e.NewRoom()
	a := newTestClient(t, room, "A", true)
	b := newTestClient(t, room, "B", true)
	a.connect()
	b.connect()
	waitTrack(t, a, "B", "video", 5)
	waitTrack(t, b, "A", "video", 5)
	b.peer.SetView(map[string]Layer{"A": LayerHigh})

	room.Close()
	a.close()
	b.close()
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<20)
			buf = buf[:runtime.Stack(buf, true)]
			t.Fatalf("goroutines: %d before, %d after\n%s", before, runtime.NumGoroutine(), buf)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestErrors(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	a := join(t, room, "A", true)

	if _, err := room.Join("A", func(Signal) {}, nil); !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("second Join: %v, want ErrAlreadyJoined", err)
	}
	if err := a.peer.HandleOffer("v=0"); !errors.Is(err, ErrBadState) {
		t.Fatalf("second HandleOffer: %v, want ErrBadState", err)
	}
	if err := a.peer.HandleAnswer("v=0"); !errors.Is(err, ErrBadState) {
		t.Fatalf("HandleAnswer without an offer: %v, want ErrBadState", err)
	}

	p, err := room.Join("junk", func(Signal) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HandleOffer("мусор"); err == nil {
		t.Fatal("HandleOffer accepted garbage")
	}

	room.Leave("A")
	if err := a.peer.HandleOffer("v=0"); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("HandleOffer after Leave: %v, want ErrRoomClosed", err)
	}
	if err := a.peer.HandleAnswer("v=0"); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("HandleAnswer after Leave: %v, want ErrRoomClosed", err)
	}

	room.Close()
	if _, err := room.Join("B", func(Signal) {}, nil); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("Join after Close: %v, want ErrRoomClosed", err)
	}
}

func TestPort(t *testing.T) {
	e := newTestEngine(t)
	if e.Port() <= 0 {
		t.Fatalf("Port = %d", e.Port())
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", e.Port()), 2*time.Second)
	if err != nil {
		t.Fatalf("tcp mux is not listening: %v", err)
	}
	_ = conn.Close()
}

func TestSenderWithoutSimulcast(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	join(t, room, "A", false)
	b := join(t, room, "B", true)
	video := waitTrack(t, b, "A", "video", 5)
	for _, p := range video.packets() {
		if p.layer != 'h' {
			t.Fatalf("got layer %q, want h (the only one)", p.layer)
		}
	}
}

func TestAnswerGoesFirst(t *testing.T) {
	e := newTestEngine(t)
	room := e.NewRoom()
	join(t, room, "A", true)
	b := join(t, room, "B", true)
	c := join(t, room, "C", true)
	// Let the renegotiation finish before the cleanup tears the clients down.
	waitTrack(t, c, "B", "audio", 1)
	waitTrack(t, b, "C", "audio", 1)

	for _, cl := range []*testClient{b, c} {
		cl.mu.Lock()
		order := append([]string(nil), cl.order...)
		cl.mu.Unlock()
		if len(order) == 0 || order[0] != "answer" {
			t.Fatalf("%s: signals %v, want the answer first", cl.id, order)
		}
	}
}

package ws

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// emit publishes an event the way a command handler does.
func (f *fixture) emit(r Renderer) {
	f.t.Helper()
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	f.hub.emitLocked(r)
}

// emitSame publishes an event for everyone the way EmitAll does.
func (f *fixture) emitSame(t string, d any) {
	f.t.Helper()
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	f.hub.emitSameLocked(t, d)
}

// onlyGM renders t and d for the GM and nothing for the others.
func onlyGM(t string, d any) Renderer {
	return func(v Viewer) (string, any, bool) { return t, d, v.IsGM }
}

func wantNoop(t *testing.T, env protocol.Envelope, seq int64) {
	t.Helper()
	if env.T != protocol.TypeNoop || env.Seq != seq || len(env.D) != 0 {
		t.Fatalf("got %+v, want sync.noop with seq %d and no data", env, seq)
	}
}

func wantEvent(t *testing.T, env protocol.Envelope, typ string, seq int64) {
	t.Helper()
	if env.T != typ || env.Seq != seq {
		t.Fatalf("got %+v, want %s with seq %d", env, typ, seq)
	}
}

// player and GM connected in this order; the player has read the GM's presence.
func connectPair(f *fixture) (player, gm *websocket.Conn) {
	f.t.Helper()
	player, _ = f.helloAs(f.player)
	gm, _ = f.helloAs(f.gm)
	readPresence(f.t, player)
	return player, gm
}

func TestRenderPerRole(t *testing.T) {
	f := newFixture(t, testOptions())
	player, gm := connectPair(f)

	f.emit(onlyGM("test.secret", map[string]int{"secret": 1}))
	n := f.stateSeq()

	env := read(t, gm)
	wantEvent(t, env, "test.secret", n)
	if string(env.D) != `{"secret":1}` {
		t.Errorf("GM data = %s", env.D)
	}
	wantNoop(t, read(t, player), n)

	f.emit(Same("test.all", 1))
	wantEvent(t, read(t, gm), "test.all", n+1)
	wantEvent(t, read(t, player), "test.all", n+1)
}

func TestRenderPerPlayer(t *testing.T) {
	f := newFixture(t, testOptions())
	other, otherID := f.addPlayer("boris")
	a, _ := f.helloAs(f.player)
	b, _ := f.helloAs(other)
	readPresence(t, a) // boris came online

	f.emit(func(v Viewer) (string, any, bool) { return "test.for_boris", 1, v.PlayerID == otherID })
	n := f.stateSeq()
	wantNoop(t, read(t, a), n)
	wantEvent(t, read(t, b), "test.for_boris", n)
}

func TestResumeRendersAnewForTheResumingConnection(t *testing.T) {
	f := newFixture(t, testOptions())
	f.helloAs(f.gm)
	// A second tab keeps the player online, so closing the first changes nothing.
	keep, _ := f.helloAs(f.player)
	_ = keep
	a1, snap := f.helloAs(f.player)
	_ = a1.Close(websocket.StatusNormalClosure, "")
	f.waitConns(f.playerID, 1)
	s := snap.Seq

	var calls atomic.Int64
	counted := func(r Renderer) Renderer {
		return func(v Viewer) (string, any, bool) {
			calls.Add(1)
			return r(v)
		}
	}
	f.emit(Same("test.one", 1))
	f.emit(counted(onlyGM("test.secret", "gm only")))
	f.emit(Same("test.three", 3))
	afterEmit := calls.Load()

	resume := func(who identity, lastSeq int64) *websocket.Conn {
		c := f.dialAs(who)
		write(t, c, fmt.Sprintf(`{"t":"sync.hello","id":"c-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, f.gs.ID, lastSeq))
		ack := read(t, c)
		if ack.T != protocol.TypeAck || !decode[protocol.HelloAck](t, ack).Resumed {
			t.Fatalf("got %+v, want ack with resumed:true", ack)
		}
		return c
	}

	p := resume(f.player, s)
	wantEvent(t, read(t, p), "test.one", s+1)
	wantNoop(t, read(t, p), s+2)
	wantEvent(t, read(t, p), "test.three", s+3)
	quiet(t, p)

	g := resume(f.gm, s)
	wantEvent(t, read(t, g), "test.one", s+1)
	if env := read(t, g); env.T != "test.secret" || env.Seq != s+2 || string(env.D) != `"gm only"` {
		t.Fatalf("GM resume got %+v, want the secret event with data", env)
	}
	wantEvent(t, read(t, g), "test.three", s+3)

	if calls.Load() <= afterEmit {
		t.Errorf("renderer calls = %d after the resumes, were %d after emit: resume must render anew", calls.Load(), afterEmit)
	}
}

// The send queue of a connection holds a whole resume buffer: before, resuming
// 999 events overflowed 64 slots and the connection was closed with 1013.
func TestResumeOfAFullBufferFits(t *testing.T) {
	opts := testOptions()
	opts.BufferSize = 0 // the default: 1000
	opts.IdleTimeout = 30 * time.Second
	opts.RatePerSec, opts.RateBurst = 1000, 1000
	f := newFixture(t, opts)

	keep, _ := f.helloAs(f.player) // keeps the player online while the other tab leaves
	_ = keep
	a1, snap := f.helloAs(f.player)
	_ = a1.Close(websocket.StatusNormalClosure, "")
	f.waitConns(f.playerID, 1)

	const n = 1000
	for i := range n {
		f.emit(Same("test.n", i))
	}

	c := f.dialAs(f.player)
	write(t, c, fmt.Sprintf(`{"t":"sync.hello","id":"c-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, f.gs.ID, snap.Seq))
	if ack := read(t, c); ack.T != protocol.TypeAck || !decode[protocol.HelloAck](t, ack).Resumed {
		t.Fatalf("got %+v, want ack with resumed:true", ack)
	}
	for i := range n {
		env := read(t, c)
		wantEvent(t, env, "test.n", snap.Seq+int64(i)+1)
	}
	quiet(t, c) // still open: not closed with 1013
}

func TestRenderEncodingErrorBecomesNoop(t *testing.T) {
	f := newFixture(t, testOptions())
	player, gm := connectPair(f)

	// A function cannot be encoded as JSON.
	f.emit(func(v Viewer) (string, any, bool) { return "test.bad", func() {}, true })
	n := f.stateSeq()
	wantNoop(t, read(t, player), n)
	wantNoop(t, read(t, gm), n)

	f.emitSame("test.bad_same", func() {})
	wantNoop(t, read(t, player), n+1)
	wantNoop(t, read(t, gm), n+1)

	f.emit(Same("test.ok", 1))
	wantEvent(t, read(t, player), "test.ok", n+2)
	wantEvent(t, read(t, gm), "test.ok", n+2)
}

func TestSameKeepsOneFrameInTheBuffer(t *testing.T) {
	f := newFixture(t, testOptions())
	f.helloAs(f.player)
	f.emitSame("test.same", 1)
	f.emit(onlyGM("test.secret", 2))

	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	events := f.hub.state.events
	last, prev := events[len(events)-1], events[len(events)-2]
	if prev.frame == nil || prev.render != nil || !strings.Contains(string(prev.frame), `"test.same"`) {
		t.Errorf("an event for everyone must keep its frame and no renderer: %+v", prev)
	}
	if last.frame != nil || last.render == nil {
		t.Errorf("a rendered event must keep its renderer and no frame: %+v", last)
	}
}

func TestNoopFrame(t *testing.T) {
	var env protocol.Envelope
	if err := json.Unmarshal(noopFrame(42), &env); err != nil || env.T != protocol.TypeNoop || env.Seq != 42 {
		t.Fatalf("noopFrame = %s, %v", noopFrame(42), err)
	}
}

// A renderer that uses Same for one of its branches is still rendered per
// recipient: the GM must not get the player's view.
func TestRendererBuiltFromSameIsStillPerRecipient(t *testing.T) {
	f := newFixture(t, testOptions())
	player, gm := connectPair(f)

	f.emit(func(v Viewer) (string, any, bool) {
		if v.IsGM {
			return "x", "gm-secret", true
		}
		return Same("x", "player-view")(v)
	})
	n := f.stateSeq()
	if env := read(t, gm); env.T != "x" || env.Seq != n || string(env.D) != `"gm-secret"` {
		t.Errorf("GM got %+v, want the secret", env)
	}
	if env := read(t, player); env.T != "x" || env.Seq != n || string(env.D) != `"player-view"` {
		t.Errorf("player got %+v, want the player view", env)
	}
}

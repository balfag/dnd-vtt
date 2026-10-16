package ws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// noLimits is testOptions with limits that commands do not run into.
func noLimits() Options {
	o := testOptions()
	o.IdleTimeout = 30 * time.Second
	o.RatePerSec, o.RateBurst = 1000, 1000
	return o
}

func (f *fixture) handle(typ string, fn Handler) {
	f.t.Helper()
	f.hub.Handle(typ, fn, HandlerOptions{})
}

func command(t *testing.T, c *websocket.Conn, typ, id string) {
	t.Helper()
	if id == "" {
		write(t, c, `{"t":"`+typ+`"}`)
		return
	}
	write(t, c, `{"t":"`+typ+`","id":"`+id+`"}`)
}

func TestCommandAckComesAfterItsEvents(t *testing.T) {
	f := newFixture(t, noLimits())
	f.handle("test.say", func(_ context.Context, _ Command, em Emitter) (any, error) {
		em.EmitAll("test.said", map[string]int{"x": 1})
		return map[string]bool{"ok": true}, nil
	})
	author, other := connectPair(f)

	command(t, author, "test.say", "c-7")
	env := read(t, author)
	if env.T != "test.said" || env.Seq == 0 {
		t.Fatalf("first message = %+v, want the event", env)
	}
	ack := read(t, author)
	if ack.T != protocol.TypeAck || ack.Re != "c-7" || string(ack.D) != `{"ok":true}` {
		t.Fatalf("second message = %+v, want the ack with data", ack)
	}

	wantEvent(t, read(t, other), "test.said", env.Seq)
	quiet(t, other) // the other client gets the event only
}

func TestCommandWithoutIDGetsNoReply(t *testing.T) {
	f := newFixture(t, noLimits())
	f.handle("test.say", func(_ context.Context, _ Command, em Emitter) (any, error) {
		em.EmitAll("test.said", 1)
		return map[string]bool{"ok": true}, nil
	})
	f.handle("test.deny", func(context.Context, Command, Emitter) (any, error) {
		return nil, Errorf("forbidden", "no")
	})
	author, _ := connectPair(f)

	command(t, author, "test.say", "")
	wantEvent(t, read(t, author), "test.said", f.stateSeq())
	command(t, author, "test.deny", "")
	quiet(t, author) // neither an ack nor an error
}

func TestCommandErrors(t *testing.T) {
	f := newFixture(t, noLimits())
	f.handle("test.missing", func(context.Context, Command, Emitter) (any, error) {
		return nil, Errorf("not_found", "scene %s", "x")
	})
	f.handle("test.boom", func(context.Context, Command, Emitter) (any, error) {
		return nil, errors.New("boom: secret detail")
	})
	f.handle("test.panic", func(context.Context, Command, Emitter) (any, error) {
		panic("handler bug")
	})
	f.handle("test.ok", func(context.Context, Command, Emitter) (any, error) { return nil, nil })
	author, _ := connectPair(f)

	command(t, author, "test.missing", "c-1")
	env := read(t, author)
	wantError(t, env, "c-1", protocol.CodeNotFound)
	if got := decode[protocol.ErrorData](t, env).Message; got != "scene x" {
		t.Errorf("message = %q, want the one of the CommandError", got)
	}

	command(t, author, "test.boom", "c-2")
	env = read(t, author)
	wantError(t, env, "c-2", protocol.CodeInternal)
	if got := decode[protocol.ErrorData](t, env).Message; strings.Contains(got, "boom") || strings.Contains(got, "secret") {
		t.Errorf("message %q leaks the error text", got)
	}

	command(t, author, "test.panic", "c-3")
	wantError(t, read(t, author), "c-3", protocol.CodeInternal)

	command(t, author, "test.ok", "c-4") // the server and the lock survived the panic
	if ack := read(t, author); ack.T != protocol.TypeAck || ack.Re != "c-4" || len(ack.D) != 0 {
		t.Fatalf("got %+v, want an ack without data", ack)
	}
}

func TestCommandWithoutActiveGame(t *testing.T) {
	f := newFixture(t, noLimits())
	var calls atomic.Int32
	f.handle("test.say", func(context.Context, Command, Emitter) (any, error) {
		calls.Add(1)
		return nil, nil
	})
	if err := f.game.EndGameSession(context.Background(), f.gs.ID); err != nil {
		t.Fatal(err)
	}
	gm, snap := f.helloAs(f.gm)
	if snap.Game != nil {
		t.Fatal("the test needs a GM outside a game")
	}

	command(t, gm, "test.say", "c-1")
	wantError(t, read(t, gm), "c-1", protocol.CodeConflict)
	if calls.Load() != 0 {
		t.Error("the handler ran without an active game")
	}
}

func TestUnknownCommandAndCommandBeforeHello(t *testing.T) {
	f := newFixture(t, noLimits())
	var calls atomic.Int32
	f.handle("test.say", func(context.Context, Command, Emitter) (any, error) {
		calls.Add(1)
		return nil, nil
	})

	c := f.dialAs(f.player)
	command(t, c, "test.say", "c-1") // a known type, but before sync.hello
	wantError(t, read(t, c), "c-1", protocol.CodeBadRequest)
	f.hello(c)
	command(t, c, "test.nothing", "c-2")
	wantError(t, read(t, c), "c-2", protocol.CodeBadRequest)
	if calls.Load() != 0 {
		t.Error("the handler ran before hello")
	}
}

func TestCommandsRunOneAtATime(t *testing.T) {
	f := newFixture(t, noLimits())
	var running, peak, total atomic.Int32
	f.handle("test.slow", func(context.Context, Command, Emitter) (any, error) {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		running.Add(-1)
		total.Add(1)
		return nil, nil
	})
	other, _ := f.addPlayer("boris")
	clients := []*websocket.Conn{}
	for _, who := range []identity{f.player, f.gm, other} {
		c, _ := f.helloAs(who)
		clients = append(clients, c)
	}

	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 5 {
				command(t, c, "test.slow", fmt.Sprintf("c-%d-%d", i, j))
			}
			acks := 0
			for acks < 5 {
				// Presence events of the other clients may come before the acks.
				if env := read(t, c); env.T == protocol.TypeAck && strings.HasPrefix(env.Re, "c-") {
					acks++
				}
			}
		}()
	}
	wg.Wait()

	if peak.Load() != 1 || total.Load() != 15 {
		t.Errorf("peak concurrency %d, commands run %d; want 1 and 15", peak.Load(), total.Load())
	}
}

func TestCommandTypeLimits(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	opts := noLimits()
	opts.Now = func() time.Time { return now } // no refill: the counts are exact
	f := newFixture(t, opts)

	var silent, loud atomic.Int32
	f.hub.Handle("test.silent", func(context.Context, Command, Emitter) (any, error) {
		silent.Add(1)
		return nil, nil
	}, HandlerOptions{RatePerSec: 5, Silent: true})
	f.hub.Handle("test.loud", func(context.Context, Command, Emitter) (any, error) {
		loud.Add(1)
		return nil, nil
	}, HandlerOptions{RatePerSec: 5})
	c, _ := f.helloAs(f.player)

	// The commands have ids, and still the rejected ones get no reply.
	for i := range 20 {
		command(t, c, "test.silent", fmt.Sprintf("s-%d", i))
	}
	for i := range 5 {
		if ack := read(t, c); ack.T != protocol.TypeAck {
			t.Fatalf("reply %d = %+v, want an ack of an executed command", i, ack)
		}
	}
	quiet(t, c) // the pong comes after all 20 are handled; no error was sent
	if silent.Load() != 5 {
		t.Errorf("silent commands run = %d, want 5", silent.Load())
	}

	for i := range 20 {
		command(t, c, "test.loud", fmt.Sprintf("l-%d", i))
	}
	acks, limited := 0, 0
	for i := range 20 {
		env := read(t, c)
		switch {
		case env.T == protocol.TypeAck:
			acks++
		case env.T == protocol.TypeError && decode[protocol.ErrorData](t, env).Code == protocol.CodeRateLimited:
			limited++
			if env.Re == "" {
				t.Errorf("reply %d has no re", i)
			}
		default:
			t.Fatalf("unexpected message %+v", env)
		}
	}
	if acks != 5 || limited != 15 || loud.Load() != 5 {
		t.Errorf("acks %d, rate_limited %d, run %d; want 5, 15, 5", acks, limited, loud.Load())
	}
}

// Rejections by the limit of a type are not a run of the general limit: even
// seven seconds of them do not close the connection with 4029.
func TestTypeLimitDoesNotFeedTheGeneralRun(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	opts := Options{
		IdleTimeout: 30 * time.Second,
		RatePerSec:  100,
		RateBurst:   1000, // above 700 commands and the hello: the general limit must never reject here
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		},
	}
	f := newFixture(t, opts)
	f.hub.Handle("test.drag", func(context.Context, Command, Emitter) (any, error) { return nil, nil },
		HandlerOptions{RatePerSec: 1, Silent: true})
	c, _ := f.helloAs(f.player)

	for range 700 {
		mu.Lock()
		now = now.Add(10 * time.Millisecond) // 7 seconds in all; the general limit refills 1 per step
		mu.Unlock()
		command(t, c, "test.drag", "")
	}
	quiet(t, c) // open: not closed with 4029
}

func TestEmitAfterTheGameChangedIsDropped(t *testing.T) {
	f := newFixture(t, noLimits())
	started, release := make(chan struct{}), make(chan struct{})
	f.handle("test.wait", func(_ context.Context, _ Command, em Emitter) (any, error) {
		close(started)
		<-release
		em.EmitAll("test.late", 1)
		em.Transient("test.late_transient", 1, nil)
		return nil, nil
	})
	gm, _ := f.helloAs(f.gm)

	command(t, gm, "test.wait", "c-1")
	<-started

	ctx := context.Background()
	if err := f.game.EndGameSession(ctx, f.gs.ID); err != nil {
		t.Fatal(err)
	}
	f.hub.GameEnded(ctx)
	if env := read(t, gm); env.T != protocol.TypeSnapshot {
		t.Fatalf("got %+v, want the snapshot without a game", env)
	}
	camp2, err := f.game.CreateCampaign(ctx, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.game.StartGameSession(ctx, camp2.ID, f.gm.acc); err != nil {
		t.Fatal(err)
	}
	f.hub.GameStarted(ctx)
	if env := read(t, gm); env.T != protocol.TypeSnapshot {
		t.Fatalf("got %+v, want the snapshot of the new game", env)
	}

	close(release)
	if ack := read(t, gm); ack.T != protocol.TypeAck || ack.Re != "c-1" {
		t.Fatalf("got %+v, want only the ack: the late events are dropped", ack)
	}
	if seq := f.stateSeq(); seq != 0 {
		t.Errorf("seq of the new game = %d, want 0", seq)
	}
	f.emit(Same("test.first", 1))
	wantEvent(t, read(t, gm), "test.first", 1)
}

func TestTransient(t *testing.T) {
	f := newFixture(t, noLimits())
	other, otherID := f.addPlayer("boris")
	f.handle("test.fly", func(_ context.Context, cmd Command, em Emitter) (any, error) {
		em.Transient("test.flying", map[string]int{"x": 1}, func(v Viewer) bool {
			return v.IsGM || v.PlayerID == otherID
		})
		return nil, nil
	})
	a, _ := f.helloAs(f.player)
	b, _ := f.helloAs(other)
	gm, _ := f.helloAs(f.gm)
	readPresence(t, a) // boris
	readPresence(t, a) // the GM
	readPresence(t, b) // the GM
	seq := f.stateSeq()

	command(t, a, "test.fly", "c-1")
	wantAckFor(t, read(t, a), "c-1") // the author is filtered out and gets only the ack
	for name, c := range map[string]*websocket.Conn{"boris": b, "gm": gm} {
		env := read(t, c)
		if env.T != "test.flying" || env.Seq != 0 || string(env.D) != `{"x":1}` {
			t.Errorf("%s got %+v, want the transient message without seq", name, env)
		}
	}
	if got := f.stateSeq(); got != seq {
		t.Errorf("seq moved from %d to %d: a transient message takes no number", seq, got)
	}

	// It is not buffered, so a resume does not bring it back.
	_ = b.Close(websocket.StatusNormalClosure, "")
	f.waitConns(otherID, 0)
	readPresence(t, a) // boris went offline
	f.emit(Same("test.after", 1))
	b2 := f.dialAs(other)
	write(t, b2, fmt.Sprintf(`{"t":"sync.hello","id":"c-2","d":{"gameSessionId":%q,"lastSeq":%d}}`, f.gs.ID, seq))
	if ack := read(t, b2); !decode[protocol.HelloAck](t, ack).Resumed {
		t.Fatalf("got %+v, want a resume", ack)
	}
	// boris online (the connection itself), boris offline were before; the events after seq follow.
	for {
		env := read(t, b2)
		if env.T == "test.after" {
			break
		}
		if env.T == "test.flying" {
			t.Fatal("a transient message was resent on resume")
		}
	}
}

func wantAckFor(t *testing.T, env protocol.Envelope, id string) {
	t.Helper()
	if env.T != protocol.TypeAck || env.Re != id {
		t.Fatalf("got %+v, want ack for %s", env, id)
	}
}

func TestHandlePanicsOnProgrammingErrors(t *testing.T) {
	f := newFixture(t, noLimits())
	fn := func(context.Context, Command, Emitter) (any, error) { return nil, nil }
	f.hub.Handle("test.once", fn, HandlerOptions{})

	for _, typ := range []string{"test.once", "", protocol.TypeHello, protocol.TypeSnapshot, "sync.anything",
		protocol.TypePing, protocol.TypePong, protocol.TypeAck, protocol.TypeError} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Handle(%q) did not panic", typ)
				}
			}()
			f.hub.Handle(typ, fn, HandlerOptions{})
		}()
	}
}

func TestCommandCarriesTheContext(t *testing.T) {
	f := newFixture(t, noLimits())
	got := make(chan Command, 1)
	f.handle("test.who", func(_ context.Context, cmd Command, _ Emitter) (any, error) {
		got <- cmd
		return nil, nil
	})
	gm, _ := f.helloAs(f.gm)
	write(t, gm, `{"t":"test.who","id":"c-1","d":{"a":1}}`)
	wantAckFor(t, read(t, gm), "c-1")

	cmd := <-got
	if cmd.T != "test.who" || cmd.ID != "c-1" || string(cmd.D) != `{"a":1}` || !cmd.Viewer.IsGM ||
		cmd.Viewer.PlayerID != f.gmPlayerID || cmd.AccountID != f.gm.acc.ID ||
		cmd.CampaignID != f.camp.ID || cmd.GameSessionID != f.gs.ID {
		t.Errorf("command = %+v", cmd)
	}
}

func TestCommandErrorMessage(t *testing.T) {
	err := Errorf("forbidden", "no %s", "way")
	if err.Error() != "ws: forbidden: no way" || err.Code != "forbidden" || err.Message != "no way" {
		t.Errorf("Errorf = %+v, %q", err, err.Error())
	}
}

// The bucket of a type holds at most RatePerSec tokens: a long pause does not
// store up more.
func TestTypeLimitBucketHasACeiling(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	opts := noLimits()
	opts.Now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	f := newFixture(t, opts)
	var ran atomic.Int32
	f.hub.Handle("test.capped", func(context.Context, Command, Emitter) (any, error) {
		ran.Add(1)
		return nil, nil
	}, HandlerOptions{RatePerSec: 5, Silent: true})
	c, _ := f.helloAs(f.player)

	command(t, c, "test.capped", "")
	quiet(t, c)
	mu.Lock()
	now = now.Add(10 * time.Second) // would refill 50 tokens without the ceiling
	mu.Unlock()
	for range 20 {
		command(t, c, "test.capped", "")
	}
	quiet(t, c)

	if got := ran.Load(); got != 1+5 {
		t.Errorf("commands run = %d, want 6 (the first and a full bucket of 5)", got)
	}
}

// An Emitter kept after its handler returned publishes nothing.
func TestEmitterIsDeadAfterTheHandlerReturns(t *testing.T) {
	f := newFixture(t, noLimits())
	saved := make(chan Emitter, 1)
	f.handle("test.keep", func(_ context.Context, _ Command, em Emitter) (any, error) {
		saved <- em
		return nil, nil
	})
	c, _ := f.helloAs(f.player)
	command(t, c, "test.keep", "c-1")
	wantAckFor(t, read(t, c), "c-1")

	em := <-saved
	seq := f.stateSeq()
	em.EmitAll("test.late", 1)
	em.Emit(Same("test.late", 2))
	em.Transient("test.late", 3, nil)

	if got := f.stateSeq(); got != seq {
		t.Errorf("seq moved from %d to %d", seq, got)
	}
	quiet(t, c) // nothing was sent
}

package ws

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// addPlayer registers one more participant of the active game.
func (f *fixture) addPlayer(login string) (identity, string) {
	f.t.Helper()
	ctx := context.Background()
	acc, p, err := f.game.RegisterPlayer(ctx, f.gs, login, playerPassword, login)
	if err != nil {
		f.t.Fatal(err)
	}
	token, _, err := f.auth.CreatePlayerSession(ctx, acc.ID, f.gs.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	a, s, err := f.auth.ResolveSession(ctx, token)
	if err != nil {
		f.t.Fatal(err)
	}
	return identity{a, s}, p.ID
}

// dialAs opens a connection for who.
func (f *fixture) dialAs(who identity) *websocket.Conn {
	f.t.Helper()
	f.as(who)
	return f.dial()
}

// helloAs opens a connection for who and completes sync.hello with a snapshot.
func (f *fixture) helloAs(who identity) (*websocket.Conn, protocol.Snapshot) {
	f.t.Helper()
	c := f.dialAs(who)
	return c, f.hello(c)
}

func (f *fixture) connsOf(playerID string) int {
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	return f.hub.countPlayerLocked(playerID)
}

// waitConns waits until the hub has n connections of the participant.
func (f *fixture) waitConns(playerID string, n int) {
	f.t.Helper()
	eventually(f.t, fmt.Sprintf("%d connections of %s", n, playerID), func() bool { return f.connsOf(playerID) == n })
}

func (f *fixture) stateSeq() int64 {
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	return f.hub.state.seq
}

// async runs fn aside and returns a function that waits for it. Hooks that
// close connections wait for the clients' close frames, so the clients must
// be able to read meanwhile.
func async(fn func()) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	return func() { <-done }
}

var pingCounter atomic.Int64

// quiet asserts that no event is pending: the next message is the pong.
func quiet(t *testing.T, c *websocket.Conn) {
	t.Helper()
	id := fmt.Sprintf("q-%d", pingCounter.Add(1))
	write(t, c, `{"t":"ping","id":"`+id+`"}`)
	if env := read(t, c); env.T != protocol.TypePong || env.Re != id {
		t.Fatalf("expected silence, got %+v", env)
	}
}

func readPresence(t *testing.T, c *websocket.Conn) (protocol.Envelope, protocol.Presence) {
	t.Helper()
	env := read(t, c)
	if env.T != protocol.TypePresence {
		t.Fatalf("got %+v, want presence.update", env)
	}
	return env, decode[protocol.Presence](t, env)
}

func (f *fixture) player0(playerID string) game.Player {
	f.t.Helper()
	players, err := f.game.ListPlayers(context.Background(), f.camp.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, p := range players {
		if p.ID == playerID {
			return p
		}
	}
	f.t.Fatalf("no player %s", playerID)
	return game.Player{}
}

func TestPresenceFollowsFirstAndLastTab(t *testing.T) {
	f := newFixture(t, testOptions())
	b, bID := f.addPlayer("boris")

	a, snapA := f.helloAs(f.player)
	quiet(t, a)

	tab1, _ := f.helloAs(b)
	env, p := readPresence(t, a)
	if p.PlayerID != bID || !p.Online || p.Player != nil {
		t.Fatalf("presence = %+v, want boris online without player data", p)
	}
	if env.Seq != snapA.Seq+1 {
		t.Errorf("seq = %d, want %d (snapshot seq + 1)", env.Seq, snapA.Seq+1)
	}

	tab2, _ := f.helloAs(b)
	quiet(t, a) // a second tab is not news
	f.waitConns(bID, 2)

	_ = tab2.Close(websocket.StatusNormalClosure, "")
	f.waitConns(bID, 1)
	quiet(t, a) // closing one of two tabs is not news

	_ = tab1.Close(websocket.StatusNormalClosure, "")
	f.waitConns(bID, 0)
	env2, p := readPresence(t, a)
	if p.PlayerID != bID || p.Online {
		t.Fatalf("presence = %+v, want boris offline", p)
	}
	if env2.Seq != env.Seq+1 {
		t.Errorf("seq = %d, want %d", env2.Seq, env.Seq+1)
	}
	quiet(t, a)
}

func TestSnapshotShowsOthersOnline(t *testing.T) {
	f := newFixture(t, testOptions())
	b, bID := f.addPlayer("boris")
	a, _ := f.helloAs(f.player)
	_ = a

	_, snapB := f.helloAs(b)
	online := map[string]bool{}
	for _, p := range snapB.Game.Players {
		online[p.ID] = p.Online
	}
	if !online[f.playerID] || !online[bID] {
		t.Errorf("online = %v, want both players online", online)
	}
}

func TestResume(t *testing.T) {
	f := newFixture(t, testOptions())
	b, bID := f.addPlayer("boris")

	// A second tab keeps A online, so A's own tab changes are not events.
	keep, _ := f.helloAs(f.player)
	_ = keep
	a1, snap := f.helloAs(f.player)
	_ = a1.Close(websocket.StatusNormalClosure, "")
	f.waitConns(f.playerID, 1)

	bc, _ := f.helloAs(b)
	_ = bc.Close(websocket.StatusNormalClosure, "")
	f.waitConns(bID, 0)

	a2 := f.dialAs(f.player)
	write(t, a2, fmt.Sprintf(`{"t":"sync.hello","id":"c-9","d":{"gameSessionId":%q,"lastSeq":%d}}`, f.gs.ID, snap.Seq))
	ack := read(t, a2)
	if ack.T != protocol.TypeAck || ack.Re != "c-9" || !decode[protocol.HelloAck](t, ack).Resumed {
		t.Fatalf("got %+v, want ack with resumed:true", ack)
	}
	for i, online := range []bool{true, false} {
		env, p := readPresence(t, a2)
		if env.Seq != snap.Seq+int64(i)+1 || p.PlayerID != bID || p.Online != online {
			t.Fatalf("event %d = %+v %+v, want seq %d, boris online=%v", i, env, p, snap.Seq+int64(i)+1, online)
		}
	}
	quiet(t, a2) // no snapshot and no further events
}

func TestResumeImpossible(t *testing.T) {
	// helloExpectSnapshot sends a hello and wants resumed:false and a snapshot.
	helloExpectSnapshot := func(t *testing.T, c *websocket.Conn, gsID string, lastSeq int64) {
		t.Helper()
		write(t, c, fmt.Sprintf(`{"t":"sync.hello","id":"c-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, gsID, lastSeq))
		ack := read(t, c)
		if ack.T != protocol.TypeAck || decode[protocol.HelloAck](t, ack).Resumed {
			t.Fatalf("got %+v, want ack with resumed:false", ack)
		}
		if env := read(t, c); env.T != protocol.TypeSnapshot {
			t.Fatalf("got %+v, want sync.snapshot", env)
		}
	}
	joins := func(f *fixture, n int) {
		p := f.player0(f.playerID)
		for range n {
			f.hub.PlayerJoined(context.Background(), p)
		}
	}

	t.Run("other game", func(t *testing.T) {
		f := newFixture(t, testOptions())
		c := f.dialAs(f.player)
		helloExpectSnapshot(t, c, "01OTHERGAME", 0)
	})
	t.Run("seq is older than the buffer", func(t *testing.T) {
		f := newFixture(t, testOptions())
		_, snap := f.helloAs(f.player)
		joins(f, 8) // BufferSize is 5
		c := f.dialAs(f.player)
		helloExpectSnapshot(t, c, f.gs.ID, snap.Seq)
	})
	t.Run("events are older than the TTL", func(t *testing.T) {
		var mu sync.Mutex
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		opts := testOptions()
		opts.Now = func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		}
		f := newFixture(t, opts)
		_, snap := f.helloAs(f.player)
		joins(f, 1)
		mu.Lock()
		now = now.Add(opts.BufferTTL + time.Second)
		mu.Unlock()
		c := f.dialAs(f.player)
		helloExpectSnapshot(t, c, f.gs.ID, snap.Seq)
	})
	t.Run("seq is ahead of the server", func(t *testing.T) {
		f := newFixture(t, testOptions())
		_, snap := f.helloAs(f.player)
		c := f.dialAs(f.player)
		helloExpectSnapshot(t, c, f.gs.ID, snap.Seq+100)
	})
}

func TestEventsKeepOrderUnderLoad(t *testing.T) {
	f := newFixture(t, testOptions())
	b, _ := f.addPlayer("boris")
	a, snapA := f.helloAs(f.player)
	bc, snapB := f.helloAs(b)

	const n = 50
	p := f.player0(f.playerID)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.hub.PlayerJoined(context.Background(), p)
		}()
	}
	wg.Wait()
	last := f.stateSeq()

	for name, tc := range map[string]struct {
		c    *websocket.Conn
		base int64
	}{"a": {a, snapA.Seq}, "b": {bc, snapB.Seq}} {
		next := tc.base + 1
		for next <= last {
			env := read(t, tc.c)
			if env.T != protocol.TypePresence {
				t.Fatalf("%s: got %+v, want presence.update", name, env)
			}
			if env.Seq != next {
				t.Fatalf("%s: seq = %d, want %d", name, env.Seq, next)
			}
			next++
		}
		if last-tc.base < n {
			t.Errorf("%s: saw %d events, want at least %d", name, last-tc.base, n)
		}
		quiet(t, tc.c)
	}
}

func TestGameEnded(t *testing.T) {
	f := newFixture(t, testOptions())
	a, _ := f.helloAs(f.player)
	gm, _ := f.helloAs(f.gm)
	// The GM's own tab announced the GM online; a hello after it sees that in the snapshot.
	quiet(t, gm)

	if err := f.game.EndGameSession(context.Background(), f.gs.ID); err != nil {
		t.Fatal(err)
	}
	wait := async(func() { f.hub.GameEnded(context.Background()) })
	defer wait()

	if got := closeStatus(t, a); got != protocol.CloseGameEnded {
		t.Fatalf("player close = %d, want 4004", got)
	}
	env := read(t, gm)
	if env.T != protocol.TypeSnapshot {
		t.Fatalf("gm got %+v, want sync.snapshot", env)
	}
	snap := decode[protocol.Snapshot](t, env)
	if snap.Game != nil || snap.Me.PlayerID != nil || snap.Seq != 0 {
		t.Errorf("snapshot = %+v, want no game, no playerId, seq 0", snap)
	}
	f.waitConns(f.playerID, 0)
	quiet(t, gm) // the player's departure is not an event of any game
}

func TestGameStarted(t *testing.T) {
	f := newFixture(t, testOptions())
	a, _ := f.helloAs(f.player)
	gm, _ := f.helloAs(f.gm)
	quiet(t, gm)

	ctx := context.Background()
	camp2, err := f.game.CreateCampaign(ctx, "Second campaign")
	if err != nil {
		t.Fatal(err)
	}
	gs2, _, err := f.game.StartGameSession(ctx, camp2.ID, f.gm.acc)
	if err != nil {
		t.Fatal(err)
	}
	gmPlayer2, err := f.game.PlayerForAccount(ctx, camp2.ID, f.gm.acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	wait := async(func() { f.hub.GameStarted(ctx) })
	defer wait()

	if got := closeStatus(t, a); got != protocol.CloseGameEnded {
		t.Fatalf("player close = %d, want 4004", got)
	}
	env := read(t, gm)
	if env.T != protocol.TypeSnapshot {
		t.Fatalf("gm got %+v, want sync.snapshot", env)
	}
	snap := decode[protocol.Snapshot](t, env)
	if snap.Game == nil || snap.Game.GameSessionID != gs2.ID || snap.Game.Campaign.Name != "Second campaign" {
		t.Fatalf("snapshot game = %+v, want the new game", snap.Game)
	}
	if snap.Seq != 0 {
		t.Errorf("seq = %d, want 0 for a new game", snap.Seq)
	}
	if snap.Me.PlayerID == nil || *snap.Me.PlayerID != gmPlayer2.ID {
		t.Errorf("me = %+v, want the gm participant of the new campaign %s", snap.Me, gmPlayer2.ID)
	}
	f.waitConns(f.playerID, 0)
	quiet(t, gm) // the old player's departure must not leak into the new game
}

func TestPlayerRemoved(t *testing.T) {
	f := newFixture(t, testOptions())
	b, bID := f.addPlayer("boris")
	a, _ := f.helloAs(f.player)
	bc, _ := f.helloAs(b)
	_, p := readPresence(t, a) // boris came online
	if p.PlayerID != bID || !p.Online {
		t.Fatalf("presence = %+v", p)
	}

	if err := f.game.RemovePlayer(context.Background(), bID); err != nil {
		t.Fatal(err)
	}
	wait := async(func() { f.hub.PlayerRemoved(bID) })
	defer wait()

	if got := closeStatus(t, bc); got != protocol.ClosePlayerRemoved {
		t.Fatalf("close = %d, want 4003", got)
	}
	_, p = readPresence(t, a)
	if p.PlayerID != bID || p.Online {
		t.Fatalf("presence = %+v, want boris offline", p)
	}
	f.waitConns(bID, 0)
	quiet(t, a) // offline was announced exactly once
}

func TestSessionRevocation(t *testing.T) {
	f := newFixture(t, testOptions())
	ctx := context.Background()
	token, _, err := f.auth.CreatePlayerSession(ctx, f.player.acc.ID, f.gs.ID)
	if err != nil {
		t.Fatal(err)
	}
	acc2, sess2, err := f.auth.ResolveSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	other := identity{acc2, sess2}
	if other.sess.ID == f.player.sess.ID {
		t.Fatal("the second login session must be different")
	}

	s1a, _ := f.helloAs(f.player)
	s1b, _ := f.helloAs(f.player)
	s2, _ := f.helloAs(other)

	wait := async(func() { f.hub.CloseSession(f.player.sess.ID) })
	for name, c := range map[string]*websocket.Conn{"first tab": s1a, "second tab": s1b} {
		if got := closeStatus(t, c); got != protocol.CloseSessionRevoked {
			t.Errorf("%s: close = %d, want 4001", name, got)
		}
	}
	wait()
	quiet(t, s2) // the other login session is alive

	s1c, _ := f.helloAs(f.player)
	wait = async(func() { f.hub.CloseAccountSessions(f.player.acc.ID, other.sess.ID) })
	defer wait()
	if got := closeStatus(t, s1c); got != protocol.CloseSessionRevoked {
		t.Errorf("tab of the other session of the account: close = %d, want 4001", got)
	}
	quiet(t, s2) // the excepted session is alive
}

func TestPlayerJoinedAnnouncesNewParticipant(t *testing.T) {
	f := newFixture(t, testOptions())
	a, _ := f.helloAs(f.player)
	_, cID := f.addPlayer("clara")

	f.hub.PlayerJoined(context.Background(), f.player0(cID))
	env, p := readPresence(t, a)
	if p.PlayerID != cID || p.Online || p.Player == nil {
		t.Fatalf("presence = %+v %+v, want clara offline with player data", env, p)
	}
	if p.Player.DisplayName != "clara" || p.Player.Role != game.RolePlayer || p.Player.Color == "" || p.Player.Online {
		t.Errorf("player = %+v", p.Player)
	}

	// A participant of another campaign is not announced.
	f.hub.PlayerJoined(context.Background(), game.Player{ID: "x", CampaignID: "other-campaign"})
	quiet(t, a)
}

// A client that never reads (frozen tab, dropped network) must not make the
// request that triggered a hook wait for the close handshake.
func TestHooksDoNotWaitForSilentClients(t *testing.T) {
	hooks := map[string]func(f *fixture){
		"CloseSession":         func(f *fixture) { f.hub.CloseSession(f.player.sess.ID) },
		"CloseAccountSessions": func(f *fixture) { f.hub.CloseAccountSessions(f.player.acc.ID, "") },
		"PlayerRemoved":        func(f *fixture) { f.hub.PlayerRemoved(f.playerID) },
		"GameEnded":            func(f *fixture) { f.hub.GameEnded(context.Background()) },
		"GameStarted":          func(f *fixture) { f.hub.GameStarted(context.Background()) },
	}
	for name, hook := range hooks {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, testOptions())
			f.helloAs(f.player) // connects, then never reads again

			start := time.Now()
			hook(f)
			if d := time.Since(start); d > time.Second {
				t.Fatalf("%s took %v, it must not wait for the close handshake", name, d)
			}
		})
	}
}

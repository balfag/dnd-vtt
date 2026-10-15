package ws

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/store"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

const (
	gmPassword     = "master-password"
	playerPassword = "player-password"
	readTimeout    = 2 * time.Second
)

type identity struct {
	acc  auth.Account
	sess auth.Session
}

type fixture struct {
	t    *testing.T
	hub  *Hub
	game *game.Service
	srv  *httptest.Server

	camp       game.Campaign
	gs         game.GameSession
	gm         identity
	player     identity
	playerID   string
	gmPlayerID string

	mu  sync.Mutex
	who identity // the identity Serve gets for the next connection
}

func testOptions() Options {
	return Options{
		IdleTimeout: 300 * time.Millisecond,
		RatePerSec:  10,
		RateBurst:   5,
		RateGrace:   200 * time.Millisecond,
		RateQuiet:   150 * time.Millisecond,
	}
}

func newFixture(t *testing.T, opts Options) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	authSvc, err := auth.NewService(st.DB(), auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	gameSvc := game.NewService(st.DB(), game.Options{})

	f := &fixture{t: t, game: gameSvc}
	setup, err := authSvc.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	gmAcc, err := authSvc.CreateGM(ctx, setup, "master", gmPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, gmSess, err := authSvc.CreateSession(ctx, gmAcc.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.gm = identity{gmAcc, gmSess}

	f.camp, err = gameSvc.CreateCampaign(ctx, "Curse of Strahd")
	if err != nil {
		t.Fatal(err)
	}
	f.gs, _, err = gameSvc.StartGameSession(ctx, f.camp.ID, gmAcc)
	if err != nil {
		t.Fatal(err)
	}
	playerAcc, player, err := gameSvc.RegisterPlayer(ctx, f.gs, "anna", playerPassword, "Anna")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authSvc.CreatePlayerSession(ctx, playerAcc.ID, f.gs.ID)
	if err != nil {
		t.Fatal(err)
	}
	acc, sess, err := authSvc.ResolveSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	f.player = identity{acc, sess}
	f.playerID = player.ID
	gmPlayer, err := gameSvc.PlayerForAccount(ctx, f.camp.ID, gmAcc.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.gmPlayerID = gmPlayer.ID

	f.hub = NewHub(gameSvc, slog.New(slog.NewTextHandler(io.Discard, nil)), opts)
	f.who = f.player
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		who := f.who
		f.mu.Unlock()
		f.hub.Serve(w, r, who.acc, who.sess)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) as(who identity) {
	f.mu.Lock()
	f.who = who
	f.mu.Unlock()
}

func (f *fixture) dial() *websocket.Conn {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.srv.URL, "http"), nil)
	if err != nil {
		f.t.Fatalf("dial: %v", err)
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	c.SetReadLimit(1 << 20) // the size test gets a 40 KiB echo
	f.t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func write(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatalf("write %.40q: %v", msg, err)
	}
}

func read(t *testing.T, c *websocket.Conn) protocol.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("bad envelope %q: %v", data, err)
	}
	return env
}

// closeStatus reads until the connection ends and returns the close code.
func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func decode[T any](t *testing.T, env protocol.Envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.D, &v); err != nil {
		t.Fatalf("decode %s data %q: %v", env.T, env.D, err)
	}
	return v
}

func (f *fixture) hello(c *websocket.Conn) protocol.Snapshot {
	f.t.Helper()
	write(f.t, c, `{"t":"sync.hello","d":{}}`)
	env := read(f.t, c)
	if env.T != protocol.TypeSnapshot {
		f.t.Fatalf("after hello got %q, want sync.snapshot", env.T)
	}
	return decode[protocol.Snapshot](f.t, env)
}

func wantError(t *testing.T, env protocol.Envelope, re, code string) {
	t.Helper()
	if env.T != protocol.TypeError || env.Re != re {
		t.Fatalf("got %+v, want error with re %q", env, re)
	}
	if got := decode[protocol.ErrorData](t, env).Code; got != code {
		t.Fatalf("error code = %q, want %q", got, code)
	}
}

func TestHelloPlayer(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	write(t, c, `{"t":"sync.hello","id":"c-1","d":{}}`)

	ack := read(t, c)
	if ack.T != protocol.TypeAck || ack.Re != "c-1" {
		t.Fatalf("first message = %+v, want ack for c-1", ack)
	}
	if decode[protocol.HelloAck](t, ack).Resumed {
		t.Error("resumed = true, want false")
	}

	env := read(t, c)
	if env.T != protocol.TypeSnapshot {
		t.Fatalf("second message = %q, want sync.snapshot", env.T)
	}
	snap := decode[protocol.Snapshot](t, env)
	if snap.Seq != 0 {
		t.Errorf("seq = %d, want 0", snap.Seq)
	}
	if snap.Me.PlayerID == nil || *snap.Me.PlayerID != f.playerID || snap.Me.IsGM || snap.Me.AccountID != f.player.acc.ID {
		t.Errorf("me = %+v, want the player %s", snap.Me, f.playerID)
	}
	if snap.Game == nil {
		t.Fatal("game = nil")
	}
	if snap.Game.GameSessionID != f.gs.ID || snap.Game.StartedAt != f.gs.StartedAt.UnixMilli() {
		t.Errorf("game = %+v, want session %s", snap.Game, f.gs.ID)
	}
	if snap.Game.Campaign.Name != "Curse of Strahd" {
		t.Errorf("campaign = %+v", snap.Game.Campaign)
	}
	online := map[string]bool{}
	for _, p := range snap.Game.Players {
		online[p.ID] = p.Online
	}
	if len(online) != 2 {
		t.Fatalf("players = %+v, want the gm and the player", snap.Game.Players)
	}
	if !online[f.playerID] {
		t.Error("the connected player is offline")
	}
	if online[f.gmPlayerID] {
		t.Error("the gm is online without a connection")
	}
}

func TestHelloWithoutIDHasNoAck(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	write(t, c, `{"t":"sync.hello"}`)
	if env := read(t, c); env.T != protocol.TypeSnapshot {
		t.Fatalf("first message = %+v, want sync.snapshot", env)
	}
}

func TestHelloRepeatedSendsNewSnapshot(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	f.hello(c)
	f.hello(c)
}

func TestHelloBadData(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	write(t, c, `{"t":"sync.hello","id":"c-1","d":"nope"}`)
	wantError(t, read(t, c), "c-1", protocol.CodeBadRequest)
}

func TestGMWithoutGame(t *testing.T) {
	f := newFixture(t, testOptions())
	if err := f.game.EndGameSession(context.Background(), f.gs.ID); err != nil {
		t.Fatal(err)
	}
	f.as(f.gm)
	c := f.dial()
	write(t, c, `{"t":"sync.hello","d":{}}`)
	env := read(t, c)
	if env.T != protocol.TypeSnapshot {
		t.Fatalf("got %+v, want sync.snapshot", env)
	}
	snap := decode[protocol.Snapshot](t, env)
	if snap.Game != nil || snap.Me.PlayerID != nil || !snap.Me.IsGM {
		t.Errorf("snapshot = %+v, want game null, playerId null, isGm", snap)
	}
	if !strings.Contains(string(env.D), `"game":null`) || !strings.Contains(string(env.D), `"playerId":null`) {
		t.Errorf("data %s must carry explicit nulls", env.D)
	}
}

func TestGMInGameHasPlayerID(t *testing.T) {
	f := newFixture(t, testOptions())
	f.as(f.gm)
	c := f.dial()
	snap := f.hello(c)
	if snap.Me.PlayerID == nil || *snap.Me.PlayerID != f.gmPlayerID {
		t.Errorf("me = %+v, want gm player %s", snap.Me, f.gmPlayerID)
	}
}

func TestCommandsBeforeHello(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()

	write(t, c, `{"t":"ping","id":"p-1"}`)
	if env := read(t, c); env.T != protocol.TypePong || env.Re != "p-1" {
		t.Fatalf("got %+v, want pong for p-1", env)
	}
	write(t, c, `{"t":"token.move","id":"c-5"}`)
	wantError(t, read(t, c), "c-5", protocol.CodeBadRequest)
}

func TestGarbage(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	f.hello(c)

	write(t, c, `not json`)
	wantError(t, read(t, c), "", protocol.CodeBadRequest)

	write(t, c, `{"t":"x","id":"c-2"}`)
	wantError(t, read(t, c), "c-2", protocol.CodeBadRequest)

	write(t, c, `{"id":"c-3"}`)
	wantError(t, read(t, c), "c-3", protocol.CodeBadRequest)

	write(t, c, `{"t":"ping","id":"p-2"}`)
	if env := read(t, c); env.T != protocol.TypePong || env.Re != "p-2" {
		t.Fatalf("connection must stay open; got %+v", env)
	}
}

func TestBinaryFrameCloses1003(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if got := closeStatus(t, c); got != websocket.StatusUnsupportedData {
		t.Fatalf("close = %d, want 1003", got)
	}
}

func TestMessageSizeLimit(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	pad := func(n int) string { return `{"t":"ping","id":"` + strings.Repeat("a", n) + `"}` }

	// 40 KiB is above the library default of 32 KiB and must pass.
	write(t, c, pad(40<<10))
	if env := read(t, c); env.T != protocol.TypePong || len(env.Re) != 40<<10 {
		t.Fatalf("40 KiB message was not answered with pong: %.60q", env.Re)
	}

	write(t, c, pad(65<<10))
	if got := closeStatus(t, c); got != websocket.StatusMessageTooBig {
		t.Fatalf("close = %d, want 1009", got)
	}
}

func TestIdleTimeout(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	if got := closeStatus(t, c); got != protocol.CloseIdle {
		t.Fatalf("close = %d, want 4008", got)
	}
}

func TestIdleTimerIsResetByMessages(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	for i := range 4 {
		time.Sleep(150 * time.Millisecond)
		write(t, c, `{"t":"ping","id":"p"}`)
		if env := read(t, c); env.T != protocol.TypePong {
			t.Fatalf("ping %d: got %+v", i, env)
		}
	}
}

// flood sends frames without pauses and reads replies in another goroutine.
func flood(t *testing.T, c *websocket.Conn, frame string, stopAfter time.Duration) (rateLimited bool, status websocket.StatusCode) {
	t.Helper()
	var (
		mu      sync.Mutex
		limited bool
		done    = make(chan websocket.StatusCode, 1)
	)
	go func() {
		for {
			_, data, err := c.Read(context.Background())
			if err != nil {
				done <- websocket.CloseStatus(err)
				return
			}
			var env protocol.Envelope
			if json.Unmarshal(data, &env) == nil && env.T == protocol.TypeError {
				mu.Lock()
				limited = limited || strings.Contains(string(env.D), protocol.CodeRateLimited)
				mu.Unlock()
			}
		}
	}()

	deadline := time.Now().Add(stopAfter)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
		err := c.Write(ctx, websocket.MessageText, []byte(frame))
		cancel()
		if err != nil {
			break
		}
		// Still far above the limit of 10/s, but slow enough for the server's
		// reply queue (64) not to overflow before RateGrace passes.
		time.Sleep(time.Millisecond)
	}
	select {
	case status = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("connection was not closed")
	}
	mu.Lock()
	defer mu.Unlock()
	return limited, status
}

func TestRateLimitClosesAfterGrace(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	limited, status := flood(t, c, `{"t":"ping","id":"p"}`, 3*time.Second)
	if !limited {
		t.Error("no rate_limited error among the replies")
	}
	if status != protocol.CloseRateLimited {
		t.Fatalf("close = %d, want 4029", status)
	}
}

func TestRateLimitCountsGarbage(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dial()
	limited, status := flood(t, c, `not json`, 3*time.Second)
	if !limited {
		t.Error("no rate_limited error among the replies")
	}
	if status != protocol.CloseRateLimited {
		t.Fatalf("close = %d, want 4029", status)
	}
}

func TestRateLimitRunEndsWithPause(t *testing.T) {
	opts := testOptions()
	// The pauses of this test are as long as the default idle timeout of the others.
	opts.IdleTimeout = 5 * time.Second
	f := newFixture(t, opts)
	c := f.dial()

	burst := func() {
		for range 10 {
			write(t, c, `{"t":"ping","id":"p"}`)
		}
	}
	burst()
	time.Sleep(300 * time.Millisecond)
	burst()
	time.Sleep(300 * time.Millisecond)
	write(t, c, `{"t":"ping","id":"last"}`)

	sawLimit := false
	for {
		env := read(t, c)
		if env.T == protocol.TypeError {
			sawLimit = true
			continue
		}
		if env.T == protocol.TypePong && env.Re == "last" {
			break
		}
	}
	if !sawLimit {
		t.Error("the bursts caused no rejections, the test proves nothing")
	}
}

func TestPlayerAfterGameEnded(t *testing.T) {
	f := newFixture(t, testOptions())
	if err := f.game.EndGameSession(context.Background(), f.gs.ID); err != nil {
		t.Fatal(err)
	}
	c := f.dial()
	if got := closeStatus(t, c); got != protocol.CloseGameEnded {
		t.Fatalf("close = %d, want 4004", got)
	}
}

func TestRemovedPlayer(t *testing.T) {
	f := newFixture(t, testOptions())
	if err := f.game.RemovePlayer(context.Background(), f.playerID); err != nil {
		t.Fatal(err)
	}
	c := f.dial()
	if got := closeStatus(t, c); got != protocol.ClosePlayerRemoved {
		t.Fatalf("close = %d, want 4003", got)
	}
}

func TestCloseAll(t *testing.T) {
	f := newFixture(t, testOptions())
	a := f.dial()
	f.as(f.gm)
	b := f.dial()
	// A snapshot proves the connection is registered.
	f.hello(a)
	f.hello(b)

	// The close handshake needs the clients to read, so CloseAll runs aside.
	closed := make(chan struct{})
	go func() {
		f.hub.CloseAll(websocket.StatusGoingAway, "bye")
		close(closed)
	}()
	for name, c := range map[string]*websocket.Conn{"player": a, "gm": b} {
		if got := closeStatus(t, c); got != websocket.StatusGoingAway {
			t.Errorf("%s: close = %d, want 1001", name, got)
		}
	}
	<-closed
	eventually(t, "all connections to be unregistered", func() bool {
		f.hub.mu.Lock()
		defer f.hub.mu.Unlock()
		return len(f.hub.conns) == 0
	})
}

func TestForeignOriginIsRejected(t *testing.T) {
	f := newFixture(t, testOptions())
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.srv.URL, "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"http://evil.example"}},
	})
	if err == nil {
		t.Fatal("handshake with a foreign Origin succeeded")
	}
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %+v, want 403", resp)
	}
}

func TestSlowClientIsClosed(t *testing.T) {
	f := newFixture(t, testOptions())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		// No writer goroutine runs, so nothing drains the queue of one.
		cn := newConn(ctx, cancel, f.hub, c, f.player.acc, f.player.sess, f.playerID, 1)
		for range 3 {
			cn.send(protocol.Envelope{T: protocol.TypePong})
		}
		<-release
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if got := closeStatus(t, c); got != websocket.StatusTryAgainLater {
		t.Fatalf("close = %d, want 1013", got)
	}
}

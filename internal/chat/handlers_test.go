package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// seqRoller returns its values in order; the test fails when they run out.
type seqRoller struct {
	t    *testing.T
	vals []int
}

func (r *seqRoller) Roll(int) int {
	r.t.Helper()
	if len(r.vals) == 0 {
		r.t.Fatal("seqRoller: out of values")
	}
	v := r.vals[0]
	r.vals = r.vals[1:]
	return v
}

type identity struct {
	acc      auth.Account
	sess     auth.Session
	playerID string
}

// table is a hub with the chat handlers, a started game, a GM and three players.
type table struct {
	t     *testing.T
	db    *sql.DB
	svc   *Service
	hub   *ws.Hub
	game  *game.Service
	auth  *auth.Service
	srv   *httptest.Server
	camp  game.Campaign
	gs    game.GameSession
	gm    identity
	anna  identity
	boris identity
	carl  identity

	mu  sync.Mutex
	who identity
}

func newTable(t *testing.T) *table {
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
	tb := &table{t: t, db: st.DB(), game: gameSvc, auth: authSvc, svc: NewService(st.DB(), Options{})}
	tb.hub = ws.NewHub(gameSvc, slog.New(slog.NewTextHandler(io.Discard, nil)), ws.Options{
		IdleTimeout: 30 * time.Second, RatePerSec: 1000, RateBurst: 1000, BufferSize: 50,
	})
	tb.hub.AddSnapshotSource(tb.svc)

	setup, err := authSvc.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	gmAcc, err := authSvc.CreateGM(ctx, setup, "master", "master-password")
	if err != nil {
		t.Fatal(err)
	}
	_, gmSess, err := authSvc.CreateSession(ctx, gmAcc.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tb.camp, err = gameSvc.CreateCampaign(ctx, "Strahd")
	if err != nil {
		t.Fatal(err)
	}
	tb.gs, _, err = gameSvc.StartGameSession(ctx, tb.camp.ID, gmAcc)
	if err != nil {
		t.Fatal(err)
	}
	tb.gm = identity{acc: gmAcc, sess: gmSess, playerID: tb.playerIDOf(gmAcc.ID)}
	tb.anna = tb.addPlayer("anna", "Anna")
	tb.boris = tb.addPlayer("boris", "Boris")
	tb.carl = tb.addPlayer("carl", "Carl")

	tb.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tb.mu.Lock()
		who := tb.who
		tb.mu.Unlock()
		tb.hub.Serve(w, r, who.acc, who.sess)
	}))
	t.Cleanup(tb.srv.Close)
	return tb
}

// register wires the chat handlers with the roller; call before the first dial.
func (tb *table) register(r *seqRoller) {
	Register(tb.hub, tb.svc, r)
}

func (tb *table) playerIDOf(accountID string) string {
	tb.t.Helper()
	players, err := tb.game.ListPlayers(context.Background(), tb.camp.ID)
	if err != nil {
		tb.t.Fatal(err)
	}
	for _, p := range players {
		if p.AccountID == accountID {
			return p.ID
		}
	}
	tb.t.Fatalf("no player of account %s", accountID)
	return ""
}

func (tb *table) addPlayer(login, name string) identity {
	tb.t.Helper()
	ctx := context.Background()
	acc, p, err := tb.game.RegisterPlayer(ctx, tb.gs, login, "player-password", name)
	if err != nil {
		tb.t.Fatal(err)
	}
	token, _, err := tb.auth.CreatePlayerSession(ctx, acc.ID, tb.gs.ID)
	if err != nil {
		tb.t.Fatal(err)
	}
	acc, sess, err := tb.auth.ResolveSession(ctx, token)
	if err != nil {
		tb.t.Fatal(err)
	}
	return identity{acc: acc, sess: sess, playerID: p.ID}
}

func (tb *table) dial(who identity) *websocket.Conn {
	tb.t.Helper()
	tb.mu.Lock()
	tb.who = who
	tb.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(tb.srv.URL, "http"), nil)
	if err != nil {
		tb.t.Fatalf("dial: %v", err)
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	c.SetReadLimit(1 << 20)
	tb.t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func recv(t *testing.T, c *websocket.Conn) protocol.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

func command(t *testing.T, c *websocket.Conn, typ, id, data string) {
	t.Helper()
	send(t, c, fmt.Sprintf(`{"t":%q,"id":%q,"d":%s}`, typ, id, data))
}

// join connects who, reads the snapshot and then the presence of everyone who
// came earlier, so that the next message of each connection is the test's own.
func (tb *table) join(conns map[string]*websocket.Conn, name string, who identity) *websocket.Conn {
	tb.t.Helper()
	c := tb.dial(who)
	send(tb.t, c, `{"t":"sync.hello","d":{}}`)
	if env := recv(tb.t, c); env.T != protocol.TypeSnapshot {
		tb.t.Fatalf("got %+v, want sync.snapshot", env)
	}
	for _, other := range conns {
		if env := recv(tb.t, other); env.T != protocol.TypePresence {
			tb.t.Fatalf("got %+v, want presence of %s", env, name)
		}
	}
	conns[name] = c
	return c
}

// everyone connects the GM, Anna, Boris and Carl.
func (tb *table) everyone() (gm, anna, boris, carl *websocket.Conn) {
	tb.t.Helper()
	conns := map[string]*websocket.Conn{}
	gm = tb.join(conns, "gm", tb.gm)
	anna = tb.join(conns, "anna", tb.anna)
	boris = tb.join(conns, "boris", tb.boris)
	carl = tb.join(conns, "carl", tb.carl)
	return gm, anna, boris, carl
}

func wantAck(t *testing.T, env protocol.Envelope, re string) {
	t.Helper()
	if env.T != protocol.TypeAck || env.Re != re {
		t.Fatalf("got %+v %s, want ack for %s", env, env.D, re)
	}
}

func wantNoop(t *testing.T, env protocol.Envelope, seq int64) {
	t.Helper()
	if env.T != protocol.TypeNoop || env.Seq != seq {
		t.Fatalf("got %+v, want sync.noop with seq %d", env, seq)
	}
}

func wantErr(t *testing.T, env protocol.Envelope, re, code string) protocol.ErrorData {
	t.Helper()
	var d protocol.ErrorData
	if env.T != protocol.TypeError || env.Re != re || json.Unmarshal(env.D, &d) != nil || d.Code != code {
		t.Fatalf("got %+v %s, want error %s for %s", env, env.D, code, re)
	}
	return d
}

// wantMessage decodes a chat.message event.
func wantMessage(t *testing.T, env protocol.Envelope) protocol.ChatMessage {
	t.Helper()
	var m protocol.ChatMessage
	if env.T != protocol.TypeChatMessage || env.Seq == 0 || json.Unmarshal(env.D, &m) != nil {
		t.Fatalf("got %+v %s, want chat.message", env, env.D)
	}
	return m
}

func (tb *table) messageCount() int {
	tb.t.Helper()
	var n int
	if err := tb.db.QueryRow(`SELECT count(*) FROM chat_message`).Scan(&n); err != nil {
		tb.t.Fatal(err)
	}
	return n
}

func TestSendPublicReachesEveryoneBeforeTheAck(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	gm, anna, boris, carl := tb.everyone()

	command(t, anna, "chat.send", "c-1", `{"text":"  Going to the door  "}`)
	own := wantMessage(t, recv(t, anna)) // the event comes before the ack
	wantAck(t, recv(t, anna), "c-1")
	if own.Text != "Going to the door" || own.Visibility != "public" || own.Kind != "text" ||
		own.AuthorPlayerID == nil || *own.AuthorPlayerID != tb.anna.playerID || len(own.To) != 0 {
		t.Errorf("message = %+v", own)
	}
	for name, c := range map[string]*websocket.Conn{"gm": gm, "boris": boris, "carl": carl} {
		if got := wantMessage(t, recv(t, c)); got.ID != own.ID {
			t.Errorf("%s got message %s, want %s", name, got.ID, own.ID)
		}
	}
	if tb.messageCount() != 1 {
		t.Errorf("stored %d messages, want 1", tb.messageCount())
	}
}

func TestPrivateMessageIsHiddenFromOthersAndFromTheGM(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	gm, anna, boris, carl := tb.everyone()

	command(t, anna, "chat.send", "c-1", fmt.Sprintf(`{"text":"Let us go around","to":[%q]}`, tb.boris.playerID))
	ev := recv(t, anna)
	m := wantMessage(t, ev)
	wantAck(t, recv(t, anna), "c-1")
	if m.Visibility != "private" || len(m.To) != 1 || m.To[0] != tb.boris.playerID {
		t.Errorf("message = %+v", m)
	}
	if got := wantMessage(t, recv(t, boris)); got.ID != m.ID {
		t.Errorf("boris got %+v", got)
	}
	wantNoop(t, recv(t, carl), ev.Seq)
	wantNoop(t, recv(t, gm), ev.Seq)
}

func TestBlindRollHidesTheResultFromItsAuthor(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t, vals: []int{14, 6}})
	gm, anna, boris, carl := tb.everyone()

	command(t, anna, "roll.request", "c-1", `{"formula":"2d20kh1+5","visibility":"blind","label":"Stealth"}`)
	own := recv(t, anna)
	wantAck(t, recv(t, anna), "c-1")
	ownMsg := wantMessage(t, own)
	if ownMsg.Roll == nil || ownMsg.Roll.Result != nil || ownMsg.Roll.Formula != "2d20kh1+5" || ownMsg.Roll.Label != "Stealth" {
		t.Errorf("author's roll = %+v", ownMsg.Roll)
	}
	gmMsg := wantMessage(t, recv(t, gm))
	if gmMsg.Roll == nil || gmMsg.Roll.Result == nil || gmMsg.Roll.Result.Total != 19 {
		t.Errorf("gm's roll = %+v", gmMsg.Roll)
	}
	wantNoop(t, recv(t, boris), own.Seq)
	wantNoop(t, recv(t, carl), own.Seq)
}

func TestGMAndSelfRolls(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t, vals: []int{3, 4}})
	gm, anna, boris, _ := tb.everyone()

	command(t, anna, "roll.request", "c-1", `{"formula":"1d20","visibility":"gm"}`)
	ev := recv(t, anna)
	wantAck(t, recv(t, anna), "c-1")
	if m := wantMessage(t, ev); m.Roll.Result == nil || m.Roll.Result.Total != 3 {
		t.Errorf("gm roll of the author = %+v", m.Roll)
	}
	if m := wantMessage(t, recv(t, gm)); m.Roll.Result == nil || m.Visibility != "gm" {
		t.Errorf("gm roll of the gm = %+v", m)
	}
	wantNoop(t, recv(t, boris), ev.Seq)

	command(t, anna, "roll.request", "c-2", `{"formula":"1d20","visibility":"self"}`)
	self := recv(t, anna)
	wantAck(t, recv(t, anna), "c-2")
	if m := wantMessage(t, self); m.Roll.Result == nil || m.Roll.Result.Total != 4 {
		t.Errorf("self roll = %+v", m.Roll)
	}
	wantNoop(t, recv(t, gm), self.Seq)
	wantNoop(t, recv(t, boris), self.Seq)
}

func TestBadFormulaIsRejectedWithTheParseError(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	gm, anna, _, _ := tb.everyone()

	command(t, anna, "roll.request", "c-1", `{"formula":"2d6+","visibility":"public"}`)
	d := wantErr(t, recv(t, anna), "c-1", protocol.CodeBadRequest)
	if !strings.HasPrefix(d.Message, "dice: ") {
		t.Errorf("message = %q, want the text of the ParseError", d.Message)
	}
	if tb.messageCount() != 0 {
		t.Errorf("stored %d messages, want 0", tb.messageCount())
	}
	// No event was emitted: the next thing a connection sees is its pong.
	for _, c := range []*websocket.Conn{anna, gm} {
		send(t, c, `{"t":"ping","id":"p-1"}`)
		if env := recv(t, c); env.T != protocol.TypePong {
			t.Errorf("got %+v, want pong", env)
		}
	}
}

func TestRollRequestIsLimitedToFivePerSecond(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t, vals: []int{1, 1, 1, 1, 1}})
	_, anna, _, _ := tb.everyone()

	for i := 1; i <= 6; i++ {
		command(t, anna, "roll.request", fmt.Sprintf("c-%d", i), `{"formula":"1d20","visibility":"self"}`)
	}
	for i := 1; i <= 5; i++ {
		wantMessage(t, recv(t, anna))
		wantAck(t, recv(t, anna), fmt.Sprintf("c-%d", i))
	}
	wantErr(t, recv(t, anna), "c-6", protocol.CodeRateLimited)
}

func TestResumeKeepsTheSeqOfAHiddenMessage(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	_, anna, boris, carl := tb.everyone()

	command(t, boris, "chat.send", "c-1", `{"text":"hello"}`)
	first := recv(t, boris)
	wantMessage(t, first)
	wantAck(t, recv(t, boris), "c-1")
	wantMessage(t, recv(t, anna))
	wantMessage(t, recv(t, carl))

	command(t, anna, "chat.send", "c-2", fmt.Sprintf(`{"text":"secret","to":[%q]}`, tb.boris.playerID))
	recv(t, anna)
	recv(t, anna)
	wantMessage(t, recv(t, boris))
	wantNoop(t, recv(t, carl), first.Seq+1)

	// Carl reconnects after the hidden message: the buffer renders it for him again.
	_ = carl.Close(websocket.StatusNormalClosure, "")
	again := tb.dial(tb.carl)
	send(t, again, fmt.Sprintf(`{"t":"sync.hello","id":"h-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, tb.gs.ID, first.Seq))
	ack := recv(t, again)
	wantAck(t, ack, "h-1")
	var resumed protocol.HelloAck
	if err := json.Unmarshal(ack.D, &resumed); err != nil || !resumed.Resumed {
		t.Fatalf("ack = %s, want a resumed hello", ack.D)
	}
	wantNoop(t, recv(t, again), first.Seq+1)
}

func TestGMWithoutPlayerIsForbidden(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	// A GM who is not a participant of the campaign has no player id.
	if _, err := tb.db.Exec(`DELETE FROM player WHERE id = ?`, tb.gm.playerID); err != nil {
		t.Fatal(err)
	}
	c := tb.dial(tb.gm)
	send(t, c, `{"t":"sync.hello","d":{}}`)
	if env := recv(t, c); env.T != protocol.TypeSnapshot {
		t.Fatalf("got %+v, want sync.snapshot", env)
	}
	command(t, c, "chat.send", "c-1", `{"text":"hi"}`)
	wantErr(t, recv(t, c), "c-1", protocol.CodeForbidden)
	command(t, c, "roll.request", "c-2", `{"formula":"1d20","visibility":"public"}`)
	wantErr(t, recv(t, c), "c-2", protocol.CodeForbidden)
	command(t, c, "chat.history", "c-3", `{"before":"x"}`)
	wantErr(t, recv(t, c), "c-3", protocol.CodeForbidden)
}

func TestHistoryOverTheWire(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	_, anna, _, _ := tb.everyone()

	var ids []string
	for i := 0; i < 3; i++ {
		command(t, anna, "chat.send", fmt.Sprintf("c-%d", i), fmt.Sprintf(`{"text":"m%d"}`, i))
		ids = append(ids, wantMessage(t, recv(t, anna)).ID)
		wantAck(t, recv(t, anna), fmt.Sprintf("c-%d", i))
	}
	command(t, anna, "chat.history", "h-1", fmt.Sprintf(`{"before":%q}`, ids[2]))
	ack := recv(t, anna)
	wantAck(t, ack, "h-1")
	var got protocol.ChatHistoryAck
	if err := json.Unmarshal(ack.D, &got); err != nil || len(got.Messages) != 2 ||
		got.Messages[0].ID != ids[0] || got.Messages[1].ID != ids[1] {
		t.Fatalf("history = %s, want %v", ack.D, ids[:2])
	}
	command(t, anna, "chat.history", "h-2", `{"before":"01J00000000000000000000XXX"}`)
	wantErr(t, recv(t, anna), "h-2", protocol.CodeNotFound)
}

package vtt

import (
	"context"
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

type identity struct {
	acc  auth.Account
	sess auth.Session
}

// table is a hub with the scene handlers, a started game, a GM and one player.
type table struct {
	t      *testing.T
	svc    *Service
	roller *sequenceRoller // rolls the initiative; set its values in a test
	hub    *ws.Hub
	game   *game.Service
	srv    *httptest.Server
	camp   game.Campaign
	gm     identity
	player identity
	boris  identity // a second player
	gs     game.GameSession

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
	tb := &table{t: t, game: gameSvc, svc: NewService(st.DB(), Options{})}
	tb.hub = ws.NewHub(gameSvc, slog.New(slog.NewTextHandler(io.Discard, nil)), ws.Options{
		IdleTimeout: 30 * time.Second, RatePerSec: 1000, RateBurst: 1000, BufferSize: 50,
	})
	tb.roller = &sequenceRoller{}
	Register(tb.hub, tb.svc, tb.roller)
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
	tb.gm = identity{gmAcc, gmSess}
	tb.camp, err = gameSvc.CreateCampaign(ctx, "Strahd")
	if err != nil {
		t.Fatal(err)
	}
	gs, _, err := gameSvc.StartGameSession(ctx, tb.camp.ID, gmAcc)
	if err != nil {
		t.Fatal(err)
	}
	pAcc, _, err := gameSvc.RegisterPlayer(ctx, gs, "anna", "player-password", "Anna")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authSvc.CreatePlayerSession(ctx, pAcc.ID, gs.ID)
	if err != nil {
		t.Fatal(err)
	}
	acc, sess, err := authSvc.ResolveSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tb.player = identity{acc, sess}
	tb.gs = gs

	bAcc, _, err := gameSvc.RegisterPlayer(ctx, gs, "boris", "player-password", "Boris")
	if err != nil {
		t.Fatal(err)
	}
	bToken, _, err := authSvc.CreatePlayerSession(ctx, bAcc.ID, gs.ID)
	if err != nil {
		t.Fatal(err)
	}
	bAcc, bSess, err := authSvc.ResolveSession(ctx, bToken)
	if err != nil {
		t.Fatal(err)
	}
	tb.boris = identity{bAcc, bSess}

	tb.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tb.mu.Lock()
		who := tb.who
		tb.mu.Unlock()
		tb.hub.Serve(w, r, who.acc, who.sess)
	}))
	t.Cleanup(tb.srv.Close)
	return tb
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

func (tb *table) hello(c *websocket.Conn) protocol.Envelope {
	tb.t.Helper()
	send(tb.t, c, `{"t":"sync.hello","d":{}}`)
	env := recv(tb.t, c)
	if env.T != protocol.TypeSnapshot {
		tb.t.Fatalf("got %+v, want sync.snapshot", env)
	}
	return env
}

func snapshotOf(t *testing.T, env protocol.Envelope) protocol.Snapshot {
	t.Helper()
	var s protocol.Snapshot
	if err := json.Unmarshal(env.D, &s); err != nil {
		t.Fatalf("snapshot %s: %v", env.D, err)
	}
	return s
}

var quietCount int

// quiet asserts that nothing is pending: the next message is the pong.
func quiet(t *testing.T, c *websocket.Conn) {
	t.Helper()
	quietCount++
	id := fmt.Sprintf("q-%d", quietCount)
	send(t, c, `{"t":"ping","id":"`+id+`"}`)
	if env := recv(t, c); env.T != protocol.TypePong || env.Re != id {
		t.Fatalf("expected silence, got %+v", env)
	}
}

func command(t *testing.T, c *websocket.Conn, typ, id, data string) {
	t.Helper()
	send(t, c, fmt.Sprintf(`{"t":%q,"id":%q,"d":%s}`, typ, id, data))
}

// pair connects the player and then the GM; the player has read the GM's presence.
func (tb *table) pair() (player, gm *websocket.Conn) {
	tb.t.Helper()
	player = tb.dial(tb.player)
	tb.hello(player)
	gm = tb.dial(tb.gm)
	tb.hello(gm)
	if env := recv(tb.t, player); env.T != protocol.TypePresence {
		tb.t.Fatalf("got %+v, want presence of the GM", env)
	}
	return player, gm
}

func wantType(t *testing.T, env protocol.Envelope, typ string) {
	t.Helper()
	if env.T != typ {
		t.Fatalf("got %+v, want %s", env, typ)
	}
}

func wantNoop(t *testing.T, env protocol.Envelope, seq int64) {
	t.Helper()
	if env.T != protocol.TypeNoop || env.Seq != seq {
		t.Fatalf("got %+v, want sync.noop with seq %d", env, seq)
	}
}

func wantErr(t *testing.T, env protocol.Envelope, re, code string) {
	t.Helper()
	var d protocol.ErrorData
	if env.T != protocol.TypeError || env.Re != re || json.Unmarshal(env.D, &d) != nil || d.Code != code {
		t.Fatalf("got %+v %s, want error %s for %s", env, env.D, code, re)
	}
}

func (tb *table) sceneCount() int {
	tb.t.Helper()
	list, err := listScenes(context.Background(), tb.dbHandle(), tb.camp.ID)
	if err != nil {
		tb.t.Fatal(err)
	}
	return len(list)
}

func (tb *table) dbHandle() queryer { return tb.svc.db }

func TestCreateScenesEmitsPerRecipient(t *testing.T) {
	tb := newTable(t)
	player, gm := tb.pair()

	command(t, gm, "scene.create", "c-1", `{"name":"Tavern"}`)
	changed := recv(t, gm)
	wantType(t, changed, protocol.TypeScenesChanged)
	activated := recv(t, gm)
	wantType(t, activated, protocol.TypeSceneActivated)
	if activated.Seq != changed.Seq+1 {
		t.Errorf("seq of scene.activated = %d, want %d", activated.Seq, changed.Seq+1)
	}
	ack := recv(t, gm)
	var created protocol.SceneCreateAck
	if ack.T != protocol.TypeAck || ack.Re != "c-1" || json.Unmarshal(ack.D, &created) != nil || created.SceneID == "" {
		t.Fatalf("got %+v %s, want the ack with sceneId", ack, ack.D)
	}

	wantNoop(t, recv(t, player), changed.Seq) // the list is for the GM
	pa := recv(t, player)
	wantType(t, pa, protocol.TypeSceneActivated)
	if pa.Seq != activated.Seq {
		t.Errorf("player seq = %d, want %d", pa.Seq, activated.Seq)
	}

	// The second scene does not change the active one.
	command(t, gm, "scene.create", "c-2", `{"name":"Crypt"}`)
	second := recv(t, gm)
	wantType(t, second, protocol.TypeScenesChanged)
	if ack := recv(t, gm); ack.T != protocol.TypeAck || ack.Re != "c-2" {
		t.Fatalf("got %+v, want the ack of c-2", ack)
	}
	wantNoop(t, recv(t, player), second.Seq)
	quiet(t, gm)
	quiet(t, player)
}

func TestUpdateSceneIsShownToPlayersOnlyWhenActive(t *testing.T) {
	tb := newTable(t)
	player, gm := tb.pair()
	ctx := context.Background()
	active, err := tb.svc.CreateScene(ctx, tb.camp.ID, protocol.SceneCreate{Name: "Active"})
	if err != nil {
		t.Fatal(err)
	}
	inactive, err := tb.svc.CreateScene(ctx, tb.camp.ID, protocol.SceneCreate{Name: "Inactive"})
	if err != nil {
		t.Fatal(err)
	}

	command(t, gm, "scene.update", "c-1", fmt.Sprintf(`{"sceneId":%q,"name":"Renamed inactive"}`, inactive.SceneID))
	for _, c := range []*websocket.Conn{gm} {
		wantType(t, recv(t, c), protocol.TypeSceneChanged)
	}
	if ack := recv(t, gm); ack.T != protocol.TypeAck || ack.Re != "c-1" {
		t.Fatalf("got %+v", ack)
	}
	wantNoop(t, recv(t, player), tb.seq())

	command(t, gm, "scene.update", "c-2", fmt.Sprintf(`{"sceneId":%q,"name":"Renamed active"}`, active.SceneID))
	wantType(t, recv(t, gm), protocol.TypeSceneChanged)
	recv(t, gm) // the ack
	env := recv(t, player)
	wantType(t, env, protocol.TypeSceneChanged)
	if strings.Contains(string(env.D), `"tokens"`) {
		t.Errorf("scene.changed carries tokens: %s", env.D)
	}
	var d protocol.SceneChanged
	if err := json.Unmarshal(env.D, &d); err != nil || d.Scene.Name != "Renamed active" || d.Scene.ID != active.SceneID {
		t.Errorf("player got %s, want the renamed active scene", env.D)
	}
}

// seq is the last seq of the active game.
func (tb *table) seq() int64 {
	tb.t.Helper()
	// The snapshot of a fresh connection carries the current seq.
	c := tb.dial(tb.gm)
	return snapshotOf(tb.t, tb.hello(c)).Seq
}

func TestBackgroundNullAndAbsentOverTheWire(t *testing.T) {
	tb := newTable(t)
	_, gm := tb.pair()
	ctx := context.Background()
	res, err := tb.svc.CreateScene(ctx, tb.camp.ID, protocol.SceneCreate{Name: "Tavern"})
	if err != nil {
		t.Fatal(err)
	}
	// The asset service is not wired here: put an asset row in directly.
	if _, err := tb.svc.db.Exec(`INSERT INTO asset (id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at)
		VALUES ('as1', ?, ?, 'image/png', 10, 640, 480, 'bg.png', 'map', 1)`, tb.camp.ID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}

	background := func(env protocol.Envelope) *protocol.AssetRef {
		t.Helper()
		var d protocol.SceneChanged
		if err := json.Unmarshal(env.D, &d); err != nil {
			t.Fatal(err)
		}
		return d.Scene.Background
	}
	update := func(id, fields string) *protocol.AssetRef {
		t.Helper()
		command(t, gm, "scene.update", id, fmt.Sprintf(`{"sceneId":%q%s}`, res.SceneID, fields))
		env := recv(t, gm)
		wantType(t, env, protocol.TypeSceneChanged)
		recv(t, gm) // the ack
		return background(env)
	}

	if ref := update("c-1", `,"backgroundAssetId":"as1"`); ref == nil || ref.AssetID != "as1" {
		t.Fatalf("background = %+v, want as1", ref)
	}
	if ref := update("c-2", `,"name":"Renamed"`); ref == nil {
		t.Error("an update without backgroundAssetId removed the background")
	}
	if ref := update("c-3", `,"backgroundAssetId":null`); ref != nil {
		t.Errorf("backgroundAssetId: null left %+v", ref)
	}
}

func TestPlayerCannotRunSceneCommands(t *testing.T) {
	tb := newTable(t)
	player, gm := tb.pair()
	first, err := tb.svc.CreateScene(context.Background(), tb.camp.ID, protocol.SceneCreate{Name: "Tavern"})
	if err != nil {
		t.Fatal(err)
	}
	seq := tb.seq()
	// The GM's connection from seq() and the previous ones saw no events; the player neither.
	id := first.SceneID
	for i, c := range []struct{ typ, data string }{
		{"scene.create", `{"name":"x"}`},
		{"scene.update", `{"sceneId":"` + id + `","name":"x"}`},
		{"scene.delete", `{"sceneId":"` + id + `"}`},
		{"scene.reorder", `{"sceneIds":["` + id + `"]}`},
		{"scene.activate", `{"sceneId":"` + id + `"}`},
		{"scene.view", `{"sceneId":"` + id + `"}`},
	} {
		rid := fmt.Sprintf("c-%d", i)
		command(t, player, c.typ, rid, c.data)
		wantErr(t, recv(t, player), rid, protocol.CodeForbidden)
	}
	if tb.sceneCount() != 1 {
		t.Errorf("scenes = %d, a player changed the database", tb.sceneCount())
	}
	if got := tb.seq(); got != seq {
		t.Errorf("seq moved from %d to %d: a refused command emitted an event", seq, got)
	}
	quiet(t, gm)
}

func TestSceneViewAnswersWithoutEvents(t *testing.T) {
	tb := newTable(t)
	player, gm := tb.pair()
	first, err := tb.svc.CreateScene(context.Background(), tb.camp.ID, protocol.SceneCreate{Name: "Tavern"})
	if err != nil {
		t.Fatal(err)
	}

	command(t, gm, "scene.view", "c-1", fmt.Sprintf(`{"sceneId":%q}`, first.SceneID))
	ack := recv(t, gm)
	var d protocol.SceneViewAck
	if ack.T != protocol.TypeAck || ack.Re != "c-1" || json.Unmarshal(ack.D, &d) != nil || d.Scene.ID != first.SceneID || d.Scene.Name != "Tavern" {
		t.Fatalf("got %+v %s", ack, ack.D)
	}
	quiet(t, gm)
	quiet(t, player)

	command(t, gm, "scene.view", "c-2", `{"sceneId":"nope"}`)
	wantErr(t, recv(t, gm), "c-2", protocol.CodeNotFound)
}

func TestSnapshotsCarryTheScenes(t *testing.T) {
	tb := newTable(t)
	ctx := context.Background()

	// Without scenes: null, an empty list, null.
	gm := tb.dial(tb.gm)
	snap := snapshotOf(t, tb.hello(gm))
	if snap.Game == nil || snap.Game.ActiveSceneID != nil || snap.Game.Scene != nil || snap.Game.Scenes == nil || len(snap.Game.Scenes) != 0 {
		t.Errorf("GM table without scenes = %+v", snap.Game)
	}

	first, err := tb.svc.CreateScene(ctx, tb.camp.ID, protocol.SceneCreate{Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := tb.svc.CreateScene(ctx, tb.camp.ID, protocol.SceneCreate{Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tb.svc.ReorderScenes(ctx, tb.camp.ID, []string{second.SceneID, first.SceneID}); err != nil {
		t.Fatal(err)
	}

	gm = tb.dial(tb.gm)
	snap = snapshotOf(t, tb.hello(gm))
	g := snap.Game
	if g.ActiveSceneID == nil || *g.ActiveSceneID != first.SceneID || g.Scene == nil || g.Scene.ID != first.SceneID {
		t.Errorf("GM active scene = %+v", g)
	}
	if len(g.Scenes) != 2 || g.Scenes[0].ID != second.SceneID || g.Scenes[1].ID != first.SceneID {
		t.Errorf("GM scenes = %+v, want them by sort", g.Scenes)
	}

	player := tb.dial(tb.player)
	env := tb.hello(player)
	if !strings.Contains(string(env.D), `"scenes":[]`) {
		t.Errorf("player snapshot %s must carry \"scenes\":[]", env.D)
	}
	snap = snapshotOf(t, env)
	if snap.Game.Scene == nil || snap.Game.Scene.ID != first.SceneID || len(snap.Game.Scenes) != 0 {
		t.Errorf("player table = %+v, want the active scene and no list", snap.Game)
	}
}

func TestResumeKeepsTheNoopOfTheSceneList(t *testing.T) {
	tb := newTable(t)
	_, gm := tb.pair()
	keep := tb.dial(tb.player) // keeps the player online while the other tab leaves
	tb.hello(keep)
	command(t, gm, "scene.create", "c-1", `{"name":"Tavern"}`)
	for range 3 { // scenes.changed, scene.activated, ack
		recv(t, gm)
	}

	p1 := tb.dial(tb.player)
	snap := snapshotOf(t, tb.hello(p1))
	last := snap.Seq
	_ = p1.Close(websocket.StatusNormalClosure, "")

	command(t, gm, "scene.create", "c-2", `{"name":"Crypt"}`)
	list := recv(t, gm)
	wantType(t, list, protocol.TypeScenesChanged)

	p2 := tb.dial(tb.player)
	send(t, p2, fmt.Sprintf(`{"t":"sync.hello","id":"c-9","d":{"gameSessionId":%q,"lastSeq":%d}}`, snap.Game.GameSessionID, last))
	if ack := recv(t, p2); ack.T != protocol.TypeAck || !strings.Contains(string(ack.D), `"resumed":true`) {
		t.Fatalf("got %+v, want a resume", ack)
	}
	wantNoop(t, recv(t, p2), list.Seq)
}

func TestDeleteActiveSceneIsAConflict(t *testing.T) {
	tb := newTable(t)
	_, gm := tb.pair()
	command(t, gm, "scene.create", "c-1", `{"name":"Tavern"}`)
	var id string
	for range 3 {
		env := recv(t, gm)
		if env.T == protocol.TypeAck {
			var a protocol.SceneCreateAck
			if err := json.Unmarshal(env.D, &a); err != nil {
				t.Fatal(err)
			}
			id = a.SceneID
		}
	}

	command(t, gm, "scene.delete", "c-2", fmt.Sprintf(`{"sceneId":%q}`, id))
	wantErr(t, recv(t, gm), "c-2", protocol.CodeConflict)
	if tb.sceneCount() != 1 {
		t.Error("the active scene was deleted")
	}
}

func TestSceneCommandsRejectBadData(t *testing.T) {
	tb := newTable(t)
	_, gm := tb.pair()

	for i, c := range []struct{ typ, data string }{
		{"scene.create", `{"name":""}`},
		{"scene.create", `{"name":"x","width":50}`},
		{"scene.create", `{"name":"x","unknown":1}`},
		{"scene.create", `"just a string"`},
		{"scene.update", `{"sceneId":"s","backgroundAssetId":5}`},
		{"scene.reorder", `{"sceneIds":["a","a"]}`},
	} {
		id := fmt.Sprintf("b-%d", i)
		command(t, gm, c.typ, id, c.data)
		env := recv(t, gm)
		if env.T != protocol.TypeError || env.Re != id {
			t.Errorf("%s %s: got %+v, want an error", c.typ, c.data, env)
			continue
		}
		var d protocol.ErrorData
		_ = json.Unmarshal(env.D, &d)
		if d.Code != protocol.CodeBadRequest && d.Code != protocol.CodeNotFound {
			t.Errorf("%s %s: code %q, want bad_request (or not_found for an unknown scene)", c.typ, c.data, d.Code)
		}
	}

	send(t, gm, `{"t":"scene.create","id":"no-data"}`)
	wantErr(t, recv(t, gm), "no-data", protocol.CodeBadRequest)
	if tb.sceneCount() != 0 {
		t.Error("a rejected command created a scene")
	}
}

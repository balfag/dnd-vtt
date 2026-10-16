package vtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// client is a connection that remembers every frame it received.
type client struct {
	t      *testing.T
	c      *websocket.Conn
	frames []string
}

func (cl *client) recv() protocol.Envelope {
	cl.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := cl.c.Read(ctx)
	if err != nil {
		cl.t.Fatalf("read: %v", err)
	}
	cl.frames = append(cl.frames, string(data))
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		cl.t.Fatalf("bad envelope %q: %v", data, err)
	}
	return env
}

func (cl *client) want(typ string) protocol.Envelope {
	cl.t.Helper()
	env := cl.recv()
	if env.T != typ {
		cl.t.Fatalf("got %+v %s, want %s", env, env.D, typ)
	}
	return env
}

func (cl *client) wantNoop(seq int64) {
	cl.t.Helper()
	wantNoop(cl.t, cl.recv(), seq)
}

func (cl *client) wantErr(re, code string) {
	cl.t.Helper()
	wantErr(cl.t, cl.recv(), re, code)
}

func (cl *client) wantAck(re string) protocol.Envelope {
	cl.t.Helper()
	env := cl.recv()
	if env.T != protocol.TypeAck || env.Re != re {
		cl.t.Fatalf("got %+v %s, want the ack of %s", env, env.D, re)
	}
	return env
}

func (cl *client) send(typ, id, data string) {
	cl.t.Helper()
	if id == "" {
		send(cl.t, cl.c, fmt.Sprintf(`{"t":%q,"d":%s}`, typ, data))
		return
	}
	command(cl.t, cl.c, typ, id, data)
}

// drainUntilPong reads until the answer to a ping and returns what came before it.
func (cl *client) drainUntilPong() []protocol.Envelope {
	cl.t.Helper()
	quietCount++
	id := fmt.Sprintf("d-%d", quietCount)
	send(cl.t, cl.c, `{"t":"ping","id":"`+id+`"}`)
	var out []protocol.Envelope
	for {
		env := cl.recv()
		if env.T == protocol.TypePong && env.Re == id {
			return out
		}
		out = append(out, env)
	}
}

func (cl *client) quiet() {
	cl.t.Helper()
	if got := cl.drainUntilPong(); len(got) != 0 {
		cl.t.Fatalf("expected silence, got %+v", got)
	}
}

func (tb *table) connect(who identity) *client {
	tb.t.Helper()
	cl := &client{t: tb.t, c: tb.dial(who)}
	send(tb.t, cl.c, `{"t":"sync.hello","d":{}}`)
	cl.want(protocol.TypeSnapshot)
	return cl
}

func (tb *table) pid(who identity) string {
	tb.t.Helper()
	p, err := tb.game.PlayerForAccount(context.Background(), tb.camp.ID, who.acc.ID)
	if err != nil {
		tb.t.Fatal(err)
	}
	return p.ID
}

// trio connects player A, player B and the GM (in this order) and reads the
// presence events they cause.
func (tb *table) trio() (a, b, gm *client) {
	tb.t.Helper()
	a = tb.connect(tb.player)
	b = tb.connect(tb.boris)
	a.want(protocol.TypePresence) // B came
	gm = tb.connect(tb.gm)
	a.want(protocol.TypePresence) // the GM came
	b.want(protocol.TypePresence)
	return a, b, gm
}

func (tb *table) scene(name string) string {
	tb.t.Helper()
	res, err := tb.svc.CreateScene(context.Background(), tb.camp.ID, protocol.SceneCreate{Name: name})
	if err != nil {
		tb.t.Fatal(err)
	}
	return res.SceneID
}

func changedToken(t *testing.T, env protocol.Envelope) protocol.Token {
	t.Helper()
	var d protocol.TokenChanged
	if env.T != protocol.TypeTokenChanged || json.Unmarshal(env.D, &d) != nil {
		t.Fatalf("got %+v %s, want token.changed", env, env.D)
	}
	return d.Token
}

func removedID(t *testing.T, env protocol.Envelope) string {
	t.Helper()
	var d protocol.TokenRef
	if env.T != protocol.TypeTokenRemoved || json.Unmarshal(env.D, &d) != nil {
		t.Fatalf("got %+v %s, want token.removed", env, env.D)
	}
	return d.TokenID
}

// createToken has the GM create a token and returns its id; the GM's own
// event and ack are read here, the players' events are left to the caller.
func createToken(t *testing.T, gm *client, id, data string) (string, protocol.Envelope) {
	t.Helper()
	gm.send("token.create", id, data)
	event := gm.recv()
	changedToken(t, event)
	var ack protocol.TokenCreateAck
	if err := json.Unmarshal(gm.wantAck(id).D, &ack); err != nil || ack.TokenID == "" {
		t.Fatalf("ack without tokenId: %v", err)
	}
	return ack.TokenID, event
}

func (tb *table) snapshotTokens(who ws.Viewer) []protocol.Token {
	tb.t.Helper()
	var tbl protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(context.Background(), who, tb.camp.ID, &tbl); err != nil {
		tb.t.Fatal(err)
	}
	if tbl.Scene == nil {
		return nil
	}
	return tbl.Scene.Tokens
}

func ids(tokens []protocol.Token) []string {
	out := []string{}
	for _, tk := range tokens {
		out = append(out, tk.ID)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestTokenCreateIsRenderedPerRecipient(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)

	data := fmt.Sprintf(`{"sceneId":%q,"x":100,"y":200,"name":"Goblin","nameVisibility":"all","owners":[%q],
		"bars":[{"key":"hp","value":5,"max":7,"visibility":"owners"},{"key":"ac","value":15,"max":15,"visibility":"all"},{"key":"sec","value":1,"max":2,"visibility":"gm"}]}`, scene, aid)
	tokenID, event := createToken(t, gm, "c-1", data)

	ea, eb := a.recv(), b.recv()
	if ea.Seq != event.Seq || eb.Seq != event.Seq {
		t.Errorf("seqs = %d, %d, GM %d; want one number for everybody", ea.Seq, eb.Seq, event.Seq)
	}
	if got := changedToken(t, event); len(got.Bars) != 3 || got.ID != tokenID {
		t.Errorf("GM token = %+v, want the full token", got)
	}
	if got := changedToken(t, ea); len(got.Bars) != 2 || got.Bars[0].Key != "hp" || got.Name != "Goblin" {
		t.Errorf("owner token = %+v, want hp and ac", got)
	}
	gotB := changedToken(t, eb)
	if len(gotB.Bars) != 1 || gotB.Bars[0].Key != "ac" {
		t.Errorf("other player's token bars = %+v, want ac only", gotB.Bars)
	}
	for _, frame := range b.frames {
		if strings.Contains(frame, `"sec"`) {
			t.Errorf("a gm bar leaked to B: %s", frame)
		}
	}
	a.quiet()
	b.quiet()
	gm.quiet()
}

func TestHiddenTokenIsKeptFromOthersAndNeverLeaks(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)

	data := fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"name":"ZZSECRETNAME","hidden":true,"owners":[%q]}`, scene, aid)
	tokenID, event := createToken(t, gm, "c-1", data)

	if got := changedToken(t, a.recv()); got.ID != tokenID || got.Name != "ZZSECRETNAME" {
		t.Errorf("the owner must see the hidden token, got %+v", got)
	}
	eb := b.recv()
	if eb.Seq != event.Seq || removedID(t, eb) != tokenID {
		t.Errorf("B got %+v %s, want token.removed of the hidden token", eb, eb.D)
	}

	if got := ids(tb.snapshotTokens(ws.Viewer{PlayerID: aid})); !contains(got, tokenID) {
		t.Errorf("snapshot of the owner lacks the token: %v", got)
	}
	if got := ids(tb.snapshotTokens(ws.Viewer{PlayerID: tb.pid(tb.boris)})); len(got) != 0 {
		t.Errorf("snapshot of B has tokens: %v", got)
	}

	// B's connection never carried the name, and the id only in token.removed.
	for _, frame := range b.frames {
		if strings.Contains(frame, "ZZSECRETNAME") {
			t.Errorf("the name leaked to B: %s", frame)
		}
		if strings.Contains(frame, tokenID) && !strings.Contains(frame, `"t":"token.removed"`) {
			t.Errorf("the id leaked to B outside token.removed: %s", frame)
		}
	}

	// Showing the token makes it appear for B.
	gm.send("token.update", "c-2", fmt.Sprintf(`{"tokenId":%q,"hidden":false}`, tokenID))
	changedToken(t, gm.recv())
	gm.wantAck("c-2")
	changedToken(t, a.recv())
	if got := changedToken(t, b.recv()); got.ID != tokenID || got.Hidden {
		t.Errorf("B got %+v, want the shown token", got)
	}
	// Hiding it again removes it.
	gm.send("token.update", "c-3", fmt.Sprintf(`{"tokenId":%q,"hidden":true}`, tokenID))
	gm.recv()
	gm.wantAck("c-3")
	a.recv()
	if removedID(t, b.recv()) != tokenID {
		t.Error("B must get token.removed when the token is hidden again")
	}
}

func TestGMLayerIsInvisibleToPlayers(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)

	data := fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"layer":"gm","name":"ZZGMNOTE","owners":[%q]}`, scene, aid)
	tokenID, _ := createToken(t, gm, "c-1", data)
	if removedID(t, a.recv()) != tokenID || removedID(t, b.recv()) != tokenID {
		t.Fatal("both players must get token.removed for a token on the gm layer, the owner included")
	}
	for _, viewer := range []ws.Viewer{{PlayerID: aid}, {PlayerID: tb.pid(tb.boris)}} {
		if got := tb.snapshotTokens(viewer); len(got) != 0 {
			t.Errorf("snapshot of %+v has %v", viewer, ids(got))
		}
	}
	if got := tb.snapshotTokens(ws.Viewer{IsGM: true}); len(got) != 1 {
		t.Errorf("GM snapshot has %d tokens, want 1", len(got))
	}
	for _, cl := range []*client{a, b} {
		for _, f := range cl.frames {
			if strings.Contains(f, "ZZGMNOTE") {
				t.Errorf("the gm note leaked: %s", f)
			}
		}
	}
}

func TestTokenOnAnInactiveSceneAppearsWithTheScene(t *testing.T) {
	tb := newTable(t)
	tb.scene("Tavern") // active
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)

	_, event := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":5,"y":5,"name":"Visible"}`, crypt))
	a.wantNoop(event.Seq)
	b.wantNoop(event.Seq)
	createToken(t, gm, "c-2", fmt.Sprintf(`{"sceneId":%q,"x":6,"y":6,"name":"ZZHIDDEN","hidden":true,"owners":[%q]}`, crypt, aid))
	a.recv()
	b.recv()
	createToken(t, gm, "c-3", fmt.Sprintf(`{"sceneId":%q,"x":7,"y":7,"name":"ZZGMLAYER","layer":"gm","owners":[%q]}`, crypt, aid))
	a.recv()
	b.recv()

	gm.send("scene.activate", "c-4", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	var sa protocol.SceneActivated
	gm.want(protocol.TypeSceneActivated)
	gm.wantAck("c-4")
	if err := json.Unmarshal(a.want(protocol.TypeSceneActivated).D, &sa); err != nil {
		t.Fatal(err)
	}
	var sb protocol.SceneActivated
	if err := json.Unmarshal(b.want(protocol.TypeSceneActivated).D, &sb); err != nil {
		t.Fatal(err)
	}
	names := func(s protocol.SceneActivated) []string {
		out := []string{}
		for _, tk := range s.Scene.Tokens {
			out = append(out, tk.Name)
		}
		return out
	}
	if got := names(sa); len(got) != 2 || got[0] != "Visible" || got[1] != "ZZHIDDEN" {
		t.Errorf("A sees %v, want Visible and ZZHIDDEN (own hidden token, no gm layer)", got)
	}
	if got := names(sb); len(got) != 1 || got[0] != "Visible" {
		t.Errorf("B sees %v, want Visible only", got)
	}
	for _, f := range b.frames {
		if strings.Contains(f, "ZZHIDDEN") || strings.Contains(f, "ZZGMLAYER") {
			t.Errorf("a token leaked to B: %s", f)
		}
	}

	// The GM views a scene with everything in it.
	gm.send("scene.view", "c-5", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	var view protocol.SceneViewAck
	if err := json.Unmarshal(gm.wantAck("c-5").D, &view); err != nil || len(view.Scene.Tokens) != 3 {
		t.Errorf("scene.view tokens = %d, %v; want 3", len(view.Scene.Tokens), err)
	}
}

func TestPlayerMovesOnlyOwnUnlockedTokens(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)

	mine, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"owners":[%q]}`, scene, aid))
	a.recv()
	b.recv()
	theirs, _ := createToken(t, gm, "c-2", fmt.Sprintf(`{"sceneId":%q,"x":20,"y":20}`, scene))
	a.recv()
	b.recv()

	a.send("token.move", "m-1", fmt.Sprintf(`{"tokenId":%q,"x":300,"y":400}`, mine))
	for _, cl := range []*client{gm, a, b} {
		if got := changedToken(t, cl.recv()); got.ID != mine || got.X != 300 || got.Y != 400 {
			t.Errorf("moved token = %+v", got)
		}
	}
	a.wantAck("m-1")

	a.send("token.move", "m-2", fmt.Sprintf(`{"tokenId":%q,"x":1,"y":1}`, theirs))
	a.wantErr("m-2", protocol.CodeForbidden)
	a.send("token.move", "m-3", fmt.Sprintf(`{"tokenId":%q,"x":99999,"y":1}`, mine))
	a.wantErr("m-3", protocol.CodeBadRequest)

	// Locked: the owner is refused, the GM moves it.
	gm.send("token.update", "c-3", fmt.Sprintf(`{"tokenId":%q,"locked":true}`, mine))
	gm.recv()
	gm.wantAck("c-3")
	a.recv()
	b.recv()
	a.send("token.move", "m-4", fmt.Sprintf(`{"tokenId":%q,"x":5,"y":5}`, mine))
	a.wantErr("m-4", protocol.CodeForbidden)
	gm.send("token.move", "c-4", fmt.Sprintf(`{"tokenId":%q,"x":6,"y":6}`, mine))
	if got := changedToken(t, gm.recv()); got.X != 6 {
		t.Errorf("GM move = %+v", got)
	}
	gm.wantAck("c-4")
	a.recv()
	b.recv()

	// Players cannot create, update or delete.
	a.send("token.create", "m-5", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":1}`, scene))
	a.wantErr("m-5", protocol.CodeForbidden)
	a.send("token.update", "m-6", fmt.Sprintf(`{"tokenId":%q,"name":"x"}`, mine))
	a.wantErr("m-6", protocol.CodeForbidden)
	a.send("token.delete", "m-7", fmt.Sprintf(`{"tokenId":%q}`, mine))
	a.wantErr("m-7", protocol.CodeForbidden)

	// A token that A cannot see does not exist for A.
	hidden, _ := createToken(t, gm, "c-5", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":1,"hidden":true}`, scene))
	a.recv()
	b.recv()
	a.send("token.move", "m-8", fmt.Sprintf(`{"tokenId":%q,"x":2,"y":2}`, hidden))
	a.wantErr("m-8", protocol.CodeNotFound)

	// Nobody but A and the GM saw the unlocked moves; the stored position is the GM's.
	got, err := tb.svc.GetToken(context.Background(), tb.camp.ID, mine)
	if err != nil || got.X != 6 || got.Y != 6 {
		t.Errorf("stored token = %+v, %v", got, err)
	}
}

func TestDragIsShownButNotStored(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)
	mine, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"owners":[%q]}`, scene, aid))
	a.recv()
	b.recv()

	a.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":50.5,"y":60}`, mine))
	for name, cl := range map[string]*client{"GM": gm, "B": b, "A": a} {
		env := cl.want(protocol.TypeTokenDragging)
		var d protocol.TokenMove
		if env.Seq != 0 || json.Unmarshal(env.D, &d) != nil || d.TokenID != mine || d.X != 50.5 || d.Y != 60 {
			t.Errorf("%s got %+v %s, want an unnumbered token.dragging", name, env, env.D)
		}
	}
	if got, _ := tb.svc.GetToken(context.Background(), tb.camp.ID, mine); got.X != 10 || got.Y != 10 {
		t.Errorf("a drag moved the stored token to %v,%v", got.X, got.Y)
	}

	// Refused drags are silent: no error, no event.
	theirs, _ := createToken(t, gm, "c-2", fmt.Sprintf(`{"sceneId":%q,"x":20,"y":20}`, scene))
	a.recv()
	b.recv()
	a.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":1,"y":1}`, theirs))
	a.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":-5,"y":1}`, mine))
	for name, cl := range map[string]*client{"A": a, "B": b, "GM": gm} {
		if got := cl.drainUntilPong(); len(got) != 0 {
			t.Errorf("%s got %+v after refused drags", name, got)
		}
	}
}

func TestDragOfAHiddenTokenIsShownToTheGMAndOwnerOnly(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	aid := tb.pid(tb.player)
	hidden, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"hidden":true,"owners":[%q]}`, scene, aid))
	a.recv()
	b.recv()

	a.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":50,"y":60}`, hidden))
	gm.want(protocol.TypeTokenDragging)
	a.want(protocol.TypeTokenDragging)
	if got := b.drainUntilPong(); len(got) != 0 {
		t.Errorf("B saw the drag of a hidden token: %+v", got)
	}
	for _, f := range b.frames {
		if strings.Contains(f, hidden) && !strings.Contains(f, `"t":"token.removed"`) {
			t.Errorf("the hidden id leaked to B: %s", f)
		}
	}
}

func TestDragRateLimitDropsSilently(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, _, gm := tb.trio()
	aid := tb.pid(tb.player)
	mine, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":10,"y":10,"owners":[%q]}`, scene, aid))
	a.recv()

	for i := range 50 {
		a.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":%d,"y":10}`, mine, i))
	}
	got := a.drainUntilPong()
	for _, env := range got {
		if env.T != protocol.TypeTokenDragging {
			t.Fatalf("A got %+v, want only token.dragging (no rate-limit errors)", env)
		}
	}
	seen := gm.drainUntilPong()
	if len(seen) > 30 || len(seen) == 0 {
		t.Errorf("GM saw %d drags of 50, want 1..30", len(seen))
	}
	a.quiet() // the connection is alive
}

func TestResumeRendersTheMissedEventsAgain(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	_, b, gm := tb.trio()
	// B's snapshot gives the point to resume from; B has read the GM's presence since.
	var env protocol.Envelope
	var snap protocol.Snapshot
	if err := json.Unmarshal([]byte(b.frames[0]), &env); err != nil || json.Unmarshal(env.D, &snap) != nil {
		t.Fatal("B's first frame is no snapshot")
	}
	last := snap.Seq + 1
	_ = b.c.CloseNow()
	gm.want(protocol.TypePresence) // B went offline

	hiddenID, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":1,"name":"ZZSECRETNAME","hidden":true}`, scene))
	shownID, _ := createToken(t, gm, "c-2", fmt.Sprintf(`{"sceneId":%q,"x":2,"y":2,"name":"Shown"}`, scene))

	b2 := &client{t: t, c: tb.dial(tb.boris)}
	send(t, b2.c, fmt.Sprintf(`{"t":"sync.hello","id":"h-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, tb.gs.ID, last))
	var replayed []protocol.Envelope
	for _, e := range b2.drainUntilPong() {
		if e.T == protocol.TypeAck && e.Re == "h-1" || e.T == protocol.TypePresence {
			continue
		}
		replayed = append(replayed, e)
	}
	if len(replayed) != 2 {
		t.Fatalf("replayed %+v, want the removal and the shown token", replayed)
	}
	if id := removedID(t, replayed[0]); id != hiddenID {
		t.Errorf("replayed event 1 = %s, want token.removed of the hidden token", id)
	}
	if got := changedToken(t, replayed[1]); got.ID != shownID || got.Name != "Shown" {
		t.Errorf("replayed event 2 = %+v", got)
	}
	for _, f := range b2.frames {
		if strings.Contains(f, "ZZSECRETNAME") {
			t.Errorf("the hidden name was replayed to B: %s", f)
		}
	}
}

func TestSnapshotOnTheWireCarriesOnlyVisibleTokens(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	ctx := context.Background()
	for i, f := range []protocol.TokenFields{
		{Name: strp("Shown")},
		{Name: strp("ZZHIDDEN"), Hidden: bp(true)},
		{Name: strp("ZZGM"), Layer: strp("gm")},
	} {
		f.X, f.Y = fp(float64(i)), fp(1)
		if _, err := tb.svc.CreateToken(ctx, tb.camp.ID, protocol.TokenCreate{SceneID: scene, TokenFields: f}); err != nil {
			t.Fatal(err)
		}
	}
	b := tb.connect(tb.boris)
	for _, f := range b.frames {
		if strings.Contains(f, "ZZHIDDEN") || strings.Contains(f, "ZZGM") || !strings.Contains(f, "Shown") {
			t.Errorf("snapshot of B = %s", f)
		}
	}
	gm := tb.connect(tb.gm)
	for _, name := range []string{"Shown", "ZZHIDDEN", "ZZGM"} {
		if !strings.Contains(gm.frames[0], name) {
			t.Errorf("the GM snapshot lacks %s: %s", name, gm.frames[0])
		}
	}
}

func TestDeleteTokenIsToldToPlayersOfTheActiveSceneOnly(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	crypt := tb.scene("Crypt")
	a, _, gm := tb.trio()

	tokenID, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":1,"hidden":true}`, scene))
	a.recv() // removed (it is hidden from A)
	gm.send("token.delete", "c-2", fmt.Sprintf(`{"tokenId":%q}`, tokenID))
	event := gm.want(protocol.TypeTokenRemoved)
	gm.wantAck("c-2")
	if got := a.want(protocol.TypeTokenRemoved); got.Seq != event.Seq || removedID(t, got) != tokenID {
		t.Errorf("A got %+v", got)
	}

	far, _ := createToken(t, gm, "c-3", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":1}`, crypt))
	a.recv()
	gm.send("token.delete", "c-4", fmt.Sprintf(`{"tokenId":%q}`, far))
	event = gm.want(protocol.TypeTokenRemoved)
	gm.wantAck("c-4")
	a.wantNoop(event.Seq)

	gm.send("token.delete", "c-5", fmt.Sprintf(`{"tokenId":%q}`, far))
	gm.wantErr("c-5", protocol.CodeNotFound)
}

func TestTokenCommandErrors(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	_, _, gm := tb.trio()
	for i, tt := range []struct {
		typ, data, code string
	}{
		{"token.create", `{"sceneId":"` + scene + `"}`, protocol.CodeBadRequest},
		{"token.create", `{"sceneId":"nope","x":1,"y":1}`, protocol.CodeNotFound},
		{"token.create", `{"sceneId":"` + scene + `","x":1,"y":1,"colour":"red"}`, protocol.CodeBadRequest},
		{"token.create", `{"sceneId":"` + scene + `","x":1,"y":1,"owners":["nobody"]}`, protocol.CodeNotFound},
		{"token.create", `{"sceneId":"` + scene + `","x":1,"y":1,"layer":"sky"}`, protocol.CodeBadRequest},
		{"token.update", `{"tokenId":"nope","name":"x"}`, protocol.CodeNotFound},
		{"token.move", `{"tokenId":"nope","x":1,"y":1}`, protocol.CodeNotFound},
		{"token.delete", `{"tokenId":"nope"}`, protocol.CodeNotFound},
	} {
		id := fmt.Sprintf("e-%d", i)
		gm.send(tt.typ, id, tt.data)
		gm.wantErr(id, tt.code)
	}
	gm.quiet()
}

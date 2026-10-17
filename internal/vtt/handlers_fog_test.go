package vtt

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

const (
	rectAround = `{"type":"rect","x":400,"y":400,"w":200,"h":200}`
	rectInside = `{"type":"rect","x":450,"y":450,"w":100,"h":100}`
	rectBig    = `{"type":"rect","x":0,"y":0,"w":1000,"h":1000}`
)

// fogChangedOf decodes a fog.changed frame and reports whether the data had a
// "tokens" field at all.
func fogChangedOf(t *testing.T, env protocol.Envelope) (d protocol.FogChanged, hasTokens bool) {
	t.Helper()
	if env.T != protocol.TypeFogChanged {
		t.Fatalf("got %+v %s, want fog.changed", env, env.D)
	}
	if err := json.Unmarshal(env.D, &d); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(env.D, &raw); err != nil {
		t.Fatal(err)
	}
	_, hasTokens = raw["tokens"]
	return d, hasTokens
}

// fogCommand sends a fog command as the GM and reads the GM's own fog.changed
// (which must have no tokens) and the ack.
func fogCommand(t *testing.T, gm *client, typ, id, data string) protocol.FogChanged {
	t.Helper()
	gm.send(typ, id, data)
	d, hasTokens := fogChangedOf(t, gm.recv())
	if hasTokens {
		t.Errorf("the GM got tokens in fog.changed")
	}
	gm.wantAck(id)
	return d
}

// playerTokens reads fog.changed as a player and returns the ids of its tokens;
// the tokens field must be there, even if empty.
func playerTokens(t *testing.T, cl *client) []string {
	t.Helper()
	d, hasTokens := fogChangedOf(t, cl.recv())
	if !hasTokens || d.Tokens == nil {
		t.Fatal("a player's fog.changed has no tokens field")
	}
	return ids(d.Tokens)
}

func sameIDs(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestFogChangesAreRenderedPerRecipient(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	crypt := tb.scene("Crypt") // not active
	a, b, gm := tb.trio()
	aid, bid := tb.pid(tb.player), tb.pid(tb.boris)

	f1, _ := createToken(t, gm, "c-1", fmt.Sprintf(`{"sceneId":%q,"x":500,"y":500,"name":"ZZF1","owners":[%q]}`, scene, aid))
	a.recv()
	b.recv()
	f2, _ := createToken(t, gm, "c-2", fmt.Sprintf(`{"sceneId":%q,"x":500,"y":500,"name":"ZZF2"}`, scene))
	a.recv()
	b.recv()

	// 1. fog.set: the GM gets no tokens, A her own token, B an empty list.
	d := fogCommand(t, gm, "fog.set", "c-4", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, scene))
	if !d.Fog.Enabled || len(d.Fog.Ops) != 0 || d.SceneID != scene {
		t.Errorf("GM fog.changed = %+v", d)
	}
	sameIDs(t, "A after fog.set", playerTokens(t, a), []string{f1})
	envB := b.recv()
	dB, hasTokens := fogChangedOf(t, envB)
	if !hasTokens || dB.Tokens == nil || len(dB.Tokens) != 0 {
		t.Errorf("B got %s, want an empty tokens array", envB.D)
	}
	if !strings.Contains(string(envB.D), `"tokens":[]`) {
		t.Errorf("tokens of B is not an empty array in %s", envB.D)
	}

	// F3 stands where nothing is ever revealed: B must never learn its name.
	f3, _ := createToken(t, gm, "c-3", fmt.Sprintf(`{"sceneId":%q,"x":2500,"y":2000,"name":"ZZSECRET"}`, scene))
	if removedID(t, a.recv()) != f3 || removedID(t, b.recv()) != f3 {
		t.Error("players must get token.removed for a token in the fog")
	}

	// 2. A reveal around the point shows F1 and F2 to B, by z.
	fogCommand(t, gm, "fog.reveal", "c-5", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround))
	sameIDs(t, "A after reveal", playerTokens(t, a), []string{f1, f2})
	sameIDs(t, "B after reveal", playerTokens(t, b), []string{f1, f2})

	// 3. A hide on top takes them away again.
	fogCommand(t, gm, "fog.hide", "c-6", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectInside))
	sameIDs(t, "A after hide", playerTokens(t, a), []string{f1})
	sameIDs(t, "B after hide", playerTokens(t, b), nil)

	// 4. A change on an inactive scene is the GM's business: players get a noop.
	gm.send("fog.set", "c-7", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, crypt))
	env := gm.recv()
	fogChangedOf(t, env)
	gm.wantAck("c-7")
	a.wantNoop(env.Seq)
	b.wantNoop(env.Seq)

	// 5. The owner walks out of the revealed area: the GM and A get
	// token.changed, B token.removed. First reveal a big area.
	fogCommand(t, gm, "fog.reveal", "c-8", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectBig))
	sameIDs(t, "A after the big reveal", playerTokens(t, a), []string{f1, f2})
	sameIDs(t, "B after the big reveal", playerTokens(t, b), []string{f1, f2})

	a.send("token.move", "c-9", fmt.Sprintf(`{"tokenId":%q,"x":2000,"y":1500}`, f1))
	if got := changedToken(t, a.recv()); got.ID != f1 || got.X != 2000 {
		t.Errorf("A got %+v, want her moved token", got)
	}
	a.wantAck("c-9")
	if got := changedToken(t, gm.recv()); got.ID != f1 {
		t.Errorf("GM got %+v", got)
	}
	if removedID(t, b.recv()) != f1 {
		t.Error("B must get token.removed when F1 leaves the revealed area")
	}

	// 6. A token in the fog is not dragged to B.
	gm.send("token.move", "c-10", fmt.Sprintf(`{"tokenId":%q,"x":2000,"y":1600}`, f2))
	changedToken(t, gm.recv())
	gm.wantAck("c-10")
	a.recv() // token.removed, F2 left the revealed area for A too
	if removedID(t, b.recv()) != f2 {
		t.Error("B must get token.removed for F2")
	}
	gm.send("token.drag", "", fmt.Sprintf(`{"tokenId":%q,"x":2100,"y":1700}`, f2))
	a.quiet()
	b.quiet()
	if got := gm.drainUntilPong(); len(got) != 1 || got[0].T != protocol.TypeTokenDragging {
		t.Errorf("the GM got %+v, want the drag", got)
	}

	// 8. The snapshot of B has the tokens of the revealed area only, the
	// fog operations are those of the GM.
	if got := ids(tb.snapshotTokens(ws.Viewer{PlayerID: bid})); len(got) != 0 {
		t.Errorf("snapshot of B has %v, want nothing: all tokens are in the fog", got)
	}
	gm.send("token.move", "c-11", fmt.Sprintf(`{"tokenId":%q,"x":600,"y":600}`, f2))
	changedToken(t, gm.recv())
	gm.wantAck("c-11")
	changedToken(t, a.recv())
	changedToken(t, b.recv())
	if got := ids(tb.snapshotTokens(ws.Viewer{PlayerID: bid})); !reflect.DeepEqual(got, []string{f2}) {
		t.Errorf("snapshot of B has %v, want only %s", got, f2)
	}
	var tblB, tblGM protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(context.Background(), ws.Viewer{PlayerID: bid}, tb.camp.ID, &tblB); err != nil {
		t.Fatal(err)
	}
	if err := tb.svc.FillSnapshot(context.Background(), ws.Viewer{IsGM: true}, tb.camp.ID, &tblGM); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tblB.Scene.Fog, tblGM.Scene.Fog) || len(tblB.Scene.Fog.Ops) != 3 {
		t.Errorf("fog in the snapshot of B = %+v, GM = %+v", tblB.Scene.Fog, tblGM.Scene.Fog)
	}
	if got := ids(tblGM.Scene.Tokens); len(got) != 3 || !hasID(got, f3) {
		t.Errorf("the GM snapshot lacks tokens: %v", got)
	}

	// 10. The name of F3 never reached B.
	for _, f := range b.frames {
		if strings.Contains(f, "ZZSECRET") || (strings.Contains(f, f3) && !strings.Contains(f, `"t":"token.removed"`)) {
			t.Errorf("F3 leaked to B: %s", f)
		}
	}
}

func TestPlayerCannotChangeTheFog(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()

	for i, c := range []struct{ typ, data string }{
		{"fog.set", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, scene)},
		{"fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround)},
		{"fog.hide", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround)},
		{"fog.reset", fmt.Sprintf(`{"sceneId":%q}`, scene)},
	} {
		id := fmt.Sprintf("c-%d", i)
		b.send(c.typ, id, c.data)
		b.wantErr(id, protocol.CodeForbidden)
	}
	a.quiet()
	b.quiet()
	gm.quiet()
}

func TestFogCommandsRejectBadData(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	_, _, gm := tb.trio()

	for _, c := range []struct{ name, typ, data, code string }{
		{"unknown scene", "fog.set", `{"sceneId":"nope","enabled":true}`, protocol.CodeNotFound},
		{"reset of an unknown scene", "fog.reset", `{"sceneId":"nope"}`, protocol.CodeNotFound},
		{"reveal on an unknown scene", "fog.reveal", fmt.Sprintf(`{"sceneId":"nope","shape":%s}`, rectAround), protocol.CodeNotFound},
		{"unknown field", "fog.set", fmt.Sprintf(`{"sceneId":%q,"extra":1}`, scene), protocol.CodeBadRequest},
		{"no shape", "fog.reveal", fmt.Sprintf(`{"sceneId":%q}`, scene), protocol.CodeBadRequest},
		{"unknown shape", "fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"star"}}`, scene), protocol.CodeBadRequest},
		{"rect with an extra field", "fog.hide", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"rect","x":0,"y":0,"w":1,"h":1,"r":2}}`, scene), protocol.CodeBadRequest},
		{"zero width", "fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"rect","x":0,"y":0,"w":0,"h":1}}`, scene), protocol.CodeBadRequest},
		{"zero radius", "fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"circle","x":0,"y":0,"r":0}}`, scene), protocol.CodeBadRequest},
		{"two points", "fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"polygon","points":[[0,0],[1,1]]}}`, scene), protocol.CodeBadRequest},
		{"far away", "fog.reveal", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"circle","x":100001,"y":0,"r":1}}`, scene), protocol.CodeBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			id := "c-" + strings.ReplaceAll(c.name, " ", "-")
			gm.send(c.typ, id, c.data)
			gm.wantErr(id, c.code)
		})
	}
	gm.quiet()
}

func TestFogOperationLimitAndReset(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	ctx := context.Background()

	shape := protocol.Shape{Type: protocol.ShapeCircle, X: 10, Y: 10, R: 5}
	for i := range maxFogOps {
		if _, err := tb.svc.AddFogOp(ctx, tb.camp.ID, "reveal", protocol.FogShape{SceneID: scene, Shape: shape}); err != nil {
			t.Fatalf("operation %d: %v", i+1, err)
		}
	}

	gm.send("fog.hide", "c-1", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround))
	gm.wantErr("c-1", protocol.CodeConflict)
	a.quiet()
	b.quiet()

	d := fogCommand(t, gm, "fog.reset", "c-2", fmt.Sprintf(`{"sceneId":%q}`, scene))
	if d.Fog.Ops == nil || len(d.Fog.Ops) != 0 {
		t.Errorf("ops after reset = %+v, want []", d.Fog.Ops)
	}
	sameIDs(t, "A after reset", playerTokens(t, a), nil)
	sameIDs(t, "B after reset", playerTokens(t, b), nil)

	d = fogCommand(t, gm, "fog.hide", "c-3", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround))
	if len(d.Fog.Ops) != 1 || d.Fog.Ops[0].Op != "hide" {
		t.Errorf("ops after adding = %+v", d.Fog.Ops)
	}
}

func TestFogResetKeepsEnabled(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	_, _, gm := tb.trio()

	fogCommand(t, gm, "fog.set", "c-1", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, scene))
	fogCommand(t, gm, "fog.reveal", "c-2", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, scene, rectAround))
	d := fogCommand(t, gm, "fog.reset", "c-3", fmt.Sprintf(`{"sceneId":%q}`, scene))
	if !d.Fog.Enabled || len(d.Fog.Ops) != 0 {
		t.Errorf("fog after reset = %+v, want enabled with no ops", d.Fog)
	}
}

func TestFogIsStoredAndShownByTheOtherViews(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	_, _, gm := tb.trio()
	ctx := context.Background()

	fogCommand(t, gm, "fog.set", "c-1", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, scene))
	fogCommand(t, gm, "fog.hide", "c-2", fmt.Sprintf(`{"sceneId":%q,"shape":{"type":"polygon","points":[[0,0],[10,0],[5,8.5]]}}`, scene))

	view, err := tb.svc.ViewScene(ctx, tb.camp.ID, scene)
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.Fog{Enabled: true, Ops: []protocol.FogOp{{Op: "hide", Shape: protocol.Shape{
		Type: protocol.ShapePolygon, Points: [][2]float64{{0, 0}, {10, 0}, {5, 8.5}},
	}}}}
	if !reflect.DeepEqual(view.Fog, want) {
		t.Errorf("stored fog = %+v, want %+v", view.Fog, want)
	}
}

func TestPlayerSeesTokenImageOnlyInTheRevealedArea(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	ctx := context.Background()
	bid := tb.pid(tb.boris)

	if _, err := tb.svc.db.Exec(`INSERT INTO asset (id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at)
		VALUES ('as9', ?, ?, 'image/png', 10, 64, 64, 'tok.png', 'token', 1)`, tb.camp.ID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.svc.CreateToken(ctx, tb.camp.ID, protocol.TokenCreate{SceneID: scene, TokenFields: protocol.TokenFields{
		X: fp(500), Y: fp(500), ImageAssetID: json.RawMessage(`"as9"`),
	}}); err != nil {
		t.Fatal(err)
	}
	check := func(when string, want bool) {
		t.Helper()
		got, err := tb.svc.PlayerCanSeeTokenImage(ctx, tb.camp.ID, bid, "as9")
		if err != nil || got != want {
			t.Errorf("%s: PlayerCanSeeTokenImage = %t, %v; want %t", when, got, err, want)
		}
	}
	check("fog off", true)
	if _, err := tb.svc.SetFog(ctx, tb.camp.ID, protocol.FogSet{SceneID: scene, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	check("fog on, nothing revealed", false)
	if _, err := tb.svc.AddFogOp(ctx, tb.camp.ID, "reveal", protocol.FogShape{SceneID: scene, Shape: protocol.Shape{
		Type: protocol.ShapeRect, X: 400, Y: 400, W: 200, H: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	check("fog on, token revealed", true)
}

func TestFogIsAppliedToTokensInSceneActivated(t *testing.T) {
	tb := newTable(t)
	tb.scene("Tavern") // active
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()

	// The fog is built on the inactive scene: nothing the players see changes.
	fogCommand(t, gm, "fog.set", "c-1", fmt.Sprintf(`{"sceneId":%q,"enabled":true}`, crypt))
	fogCommand(t, gm, "fog.reveal", "c-2", fmt.Sprintf(`{"sceneId":%q,"shape":%s}`, crypt, rectAround))
	seen, _ := createToken(t, gm, "c-3", fmt.Sprintf(`{"sceneId":%q,"x":500,"y":500,"name":"ZZSEEN"}`, crypt))
	fogged, _ := createToken(t, gm, "c-4", fmt.Sprintf(`{"sceneId":%q,"x":2500,"y":2000,"name":"ZZFOGGED"}`, crypt))

	for range 4 { // sync.noop for each change on the inactive scene
		a.recv()
		b.recv()
	}

	gm.send("scene.activate", "c-5", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	var sg protocol.SceneActivated
	if err := json.Unmarshal(gm.want(protocol.TypeSceneActivated).D, &sg); err != nil {
		t.Fatal(err)
	}
	gm.wantAck("c-5")
	sameIDs(t, "GM tokens", ids(sg.Scene.Tokens), []string{seen, fogged})

	for name, cl := range map[string]*client{"A": a, "B": b} {
		var s protocol.SceneActivated
		if err := json.Unmarshal(cl.want(protocol.TypeSceneActivated).D, &s); err != nil {
			t.Fatal(err)
		}
		sameIDs(t, name+" tokens", ids(s.Scene.Tokens), []string{seen})
		if !s.Scene.Fog.Enabled {
			t.Errorf("%s: fog is not enabled in scene.activated", name)
		}
		for _, f := range cl.frames {
			if strings.Contains(f, fogged) || strings.Contains(f, "ZZFOGGED") {
				t.Errorf("a token in the fog leaked to %s: %s", name, f)
			}
		}
	}
}

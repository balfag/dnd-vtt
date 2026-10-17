package vtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

const testRect = `{"x":100,"y":150,"w":200,"h":120}`

func changedDrawing(t *testing.T, env protocol.Envelope) protocol.Drawing {
	t.Helper()
	var d protocol.DrawingChanged
	if env.T != protocol.TypeDrawingChanged || json.Unmarshal(env.D, &d) != nil {
		t.Fatalf("got %+v %s, want drawing.changed", env, env.D)
	}
	return d.Drawing
}

func removedDrawingID(t *testing.T, env protocol.Envelope) string {
	t.Helper()
	var d protocol.DrawingRef
	if env.T != protocol.TypeDrawingRemoved || json.Unmarshal(env.D, &d) != nil {
		t.Fatalf("got %+v %s, want drawing.removed", env, env.D)
	}
	return d.DrawingID
}

func drawCmd(sceneID, kind, data string) string {
	return fmt.Sprintf(`{"sceneId":%q,"kind":%q,"data":%s,"color":"#e53935"}`, sceneID, kind, data)
}

// draw has c create a drawing it may create and reads its own event and ack;
// the events of the others are left to the caller.
func draw(t *testing.T, c *client, id, sceneID, kind, data string) string {
	t.Helper()
	c.send("drawing.create", id, drawCmd(sceneID, kind, data))
	changedDrawing(t, c.recv())
	var ack protocol.DrawingCreateAck
	if err := json.Unmarshal(c.wantAck(id).D, &ack); err != nil || ack.DrawingID == "" {
		t.Fatalf("ack without drawingId: %v", err)
	}
	return ack.DrawingID
}

func TestDrawingIsShownToEveryoneOnTheActiveScene(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()

	a.send("drawing.create", "c-1", drawCmd(scene, "rect", `{ "h": 120, "w": 200, "y": 150, "x": 100 }`))
	got := changedDrawing(t, a.recv()) // the event comes before the ack
	var ack protocol.DrawingCreateAck
	if err := json.Unmarshal(a.wantAck("c-1").D, &ack); err != nil || ack.DrawingID != got.ID {
		t.Fatalf("ack %+v, err %v, want the id %s", ack, err, got.ID)
	}
	if got.AuthorPlayerID != tb.pid(tb.player) || got.SceneID != scene || got.Kind != "rect" || got.Color != "#e53935" {
		t.Errorf("drawing %+v", got)
	}
	if string(got.Data) != testRect {
		t.Errorf("data %s, want the normalized %s", got.Data, testRect)
	}
	for _, c := range []*client{b, gm} {
		if other := changedDrawing(t, c.recv()); other.ID != got.ID || other.AuthorPlayerID != got.AuthorPlayerID {
			t.Errorf("other recipient got %+v", other)
		}
	}
}

func TestDrawingsOfEveryKindAreCreated(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, _, gm := tb.trio()
	for i, tc := range []struct{ kind, data string }{
		{"freehand", `{"points":[[1,2],[3,4]]}`},
		{"rect", testRect},
		{"circle", `{"x":1,"y":2,"r":3}`},
		{"text", `{"x":1,"y":2,"text":"Ловушка","size":24}`},
		{"template", `{"shape":"cone","x":1,"y":2,"size":15,"direction":-90}`},
	} {
		id := fmt.Sprintf("c-%d", i+1)
		draw(t, a, id, scene, tc.kind, tc.data)
		if got := changedDrawing(t, gm.recv()); got.Kind != tc.kind {
			t.Errorf("got %+v, want kind %s", got, tc.kind)
		}
	}
}

func TestOnlyTheAuthorOrTheGMChangesADrawing(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()
	id := draw(t, a, "c-1", scene, "rect", testRect)
	b.want(protocol.TypeDrawingChanged)
	gm.want(protocol.TypeDrawingChanged)

	// B is not the author: neither update nor delete.
	b.send("drawing.update", "c-2", fmt.Sprintf(`{"drawingId":%q,"color":"#000000"}`, id))
	b.wantErr("c-2", protocol.CodeForbidden)
	b.send("drawing.delete", "c-3", fmt.Sprintf(`{"drawingId":%q}`, id))
	b.wantErr("c-3", protocol.CodeForbidden)
	a.quiet()
	gm.quiet()

	// The GM may.
	gm.send("drawing.update", "c-4", fmt.Sprintf(`{"drawingId":%q,"color":"#000000","data":{"x":1,"y":2,"w":3,"h":4}}`, id))
	for _, c := range []*client{gm, a, b} {
		got := changedDrawing(t, c.recv())
		if got.Color != "#000000" || string(got.Data) != `{"x":1,"y":2,"w":3,"h":4}` || got.AuthorPlayerID != tb.pid(tb.player) {
			t.Errorf("got %+v", got)
		}
	}
	gm.wantAck("c-4")

	// The author may change and delete their own.
	a.send("drawing.update", "c-5", fmt.Sprintf(`{"drawingId":%q,"color":"#43a047"}`, id))
	if got := changedDrawing(t, a.recv()); got.Color != "#43a047" || string(got.Data) != `{"x":1,"y":2,"w":3,"h":4}` {
		t.Errorf("a color change must keep the data: %+v", got)
	}
	a.wantAck("c-5")
	b.want(protocol.TypeDrawingChanged)
	gm.want(protocol.TypeDrawingChanged)

	a.send("drawing.delete", "c-6", fmt.Sprintf(`{"drawingId":%q}`, id))
	for _, c := range []*client{a, b, gm} {
		if got := removedDrawingID(t, c.recv()); got != id {
			t.Errorf("removed %s, want %s", got, id)
		}
	}
	a.wantAck("c-6")
	a.send("drawing.delete", "c-7", fmt.Sprintf(`{"drawingId":%q}`, id))
	a.wantErr("c-7", protocol.CodeNotFound)
}

func TestDrawingOnAnInactiveScene(t *testing.T) {
	tb := newTable(t)
	tb.scene("Tavern") // active
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()

	// The GM may; the players learn nothing.
	gm.send("drawing.create", "c-1", drawCmd(crypt, "circle", `{"x":1,"y":2,"r":3}`))
	ev := gm.recv()
	id := changedDrawing(t, ev).ID
	gm.wantAck("c-1")
	a.wantNoop(ev.Seq)
	b.wantNoop(ev.Seq)

	gm.send("drawing.update", "c-2", fmt.Sprintf(`{"drawingId":%q,"color":"#000000"}`, id))
	ev = gm.recv()
	gm.wantAck("c-2")
	a.wantNoop(ev.Seq)
	b.wantNoop(ev.Seq)

	// A player may not draw there.
	a.send("drawing.create", "c-3", drawCmd(crypt, "circle", `{"x":1,"y":2,"r":3}`))
	a.wantErr("c-3", protocol.CodeForbidden)
	gm.quiet()

	gm.send("drawing.delete", "c-4", fmt.Sprintf(`{"drawingId":%q}`, id))
	ev = gm.recv()
	removedDrawingID(t, ev)
	gm.wantAck("c-4")
	a.wantNoop(ev.Seq)
	b.wantNoop(ev.Seq)
}

func TestAuthorCannotChangeADrawingOfAScenePutAway(t *testing.T) {
	tb := newTable(t)
	tavern := tb.scene("Tavern")
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()
	id := draw(t, a, "c-1", tavern, "rect", testRect)
	b.want(protocol.TypeDrawingChanged)
	gm.want(protocol.TypeDrawingChanged)

	gm.send("scene.activate", "c-2", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	for _, c := range []*client{gm, a, b} {
		c.want(protocol.TypeSceneActivated)
	}
	gm.wantAck("c-2")

	a.send("drawing.update", "c-3", fmt.Sprintf(`{"drawingId":%q,"color":"#000000"}`, id))
	a.wantErr("c-3", protocol.CodeForbidden)
	a.send("drawing.delete", "c-4", fmt.Sprintf(`{"drawingId":%q}`, id))
	a.wantErr("c-4", protocol.CodeForbidden)
}

func TestDrawingCommandsRejectBadData(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, _, gm := tb.trio()
	id := draw(t, a, "c-0", scene, "text", `{"x":1,"y":2,"text":"a","size":16}`)
	gm.want(protocol.TypeDrawingChanged)

	for i, tc := range []struct {
		name, typ, data string
		code            string
	}{
		{"unknown kind", "drawing.create", drawCmd(scene, "arrow", testRect), protocol.CodeBadRequest},
		{"data of another kind", "drawing.create", drawCmd(scene, "circle", testRect), protocol.CodeBadRequest},
		{"upper case color", "drawing.create", fmt.Sprintf(`{"sceneId":%q,"kind":"rect","data":%s,"color":"#E53935"}`, scene, testRect), protocol.CodeBadRequest},
		{"short color", "drawing.create", fmt.Sprintf(`{"sceneId":%q,"kind":"rect","data":%s,"color":"#fff"}`, scene, testRect), protocol.CodeBadRequest},
		{"no data", "drawing.create", fmt.Sprintf(`{"sceneId":%q,"kind":"rect","color":"#e53935"}`, scene), protocol.CodeBadRequest},
		{"unknown field", "drawing.create", fmt.Sprintf(`{"sceneId":%q,"kind":"rect","data":%s,"color":"#e53935","x":1}`, scene, testRect), protocol.CodeBadRequest},
		{"unknown scene", "drawing.create", drawCmd("nope", "rect", testRect), protocol.CodeNotFound},
		{"points given to a text", "drawing.update", fmt.Sprintf(`{"drawingId":%q,"data":{"points":[[1,2],[3,4]]}}`, id), protocol.CodeBadRequest},
		{"text out of range", "drawing.update", fmt.Sprintf(`{"drawingId":%q,"data":{"x":1,"y":2,"text":"a","size":7}}`, id), protocol.CodeBadRequest},
		{"null data", "drawing.update", fmt.Sprintf(`{"drawingId":%q,"data":null}`, id), protocol.CodeBadRequest},
		{"bad color", "drawing.update", fmt.Sprintf(`{"drawingId":%q,"color":"red"}`, id), protocol.CodeBadRequest},
		{"unknown drawing", "drawing.update", `{"drawingId":"nope","color":"#000000"}`, protocol.CodeNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cid := fmt.Sprintf("e-%d", i)
			a.send(tc.typ, cid, tc.data)
			a.wantErr(cid, tc.code)
		})
	}
	a.quiet()
	gm.quiet()
}

func TestDrawingsAreInTheSceneViews(t *testing.T) {
	tb := newTable(t)
	tavern := tb.scene("Tavern")
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()
	onTavern := draw(t, a, "c-1", tavern, "rect", testRect)
	b.want(protocol.TypeDrawingChanged)
	gm.want(protocol.TypeDrawingChanged)
	gm.send("drawing.create", "c-2", drawCmd(crypt, "circle", `{"x":1,"y":2,"r":3}`))
	ev := gm.recv()
	onCrypt := changedDrawing(t, ev).ID
	gm.wantAck("c-2")
	a.wantNoop(ev.Seq)
	b.wantNoop(ev.Seq)

	// The snapshot carries the drawings of the active scene.
	snap := snapshotOf(t, tb.hello(tb.dial(tb.boris)))
	if ids := drawingIDs(snap.Game.Scene.Drawings); len(ids) != 1 || ids[0] != onTavern {
		t.Errorf("snapshot drawings %v, want [%s]", ids, onTavern)
	}

	// scene.view gives the GM the drawings of any scene.
	gm.send("scene.view", "c-3", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	var view protocol.SceneViewAck
	if err := json.Unmarshal(gm.wantAck("c-3").D, &view); err != nil {
		t.Fatal(err)
	}
	if ids := drawingIDs(view.Scene.Drawings); len(ids) != 1 || ids[0] != onCrypt {
		t.Errorf("scene.view drawings %v, want [%s]", ids, onCrypt)
	}

	// scene.activated carries them to everyone, scene.changed to nobody.
	gm.send("scene.update", "c-4", fmt.Sprintf(`{"sceneId":%q,"name":"Tavern 2"}`, tavern))
	changed := gm.want(protocol.TypeSceneChanged)
	gm.wantAck("c-4")
	a.want(protocol.TypeSceneChanged)
	b.want(protocol.TypeSceneChanged)
	if strings.Contains(string(changed.D), "drawings") {
		t.Errorf("scene.changed carries drawings: %s", changed.D)
	}
	gm.send("scene.activate", "c-5", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	for _, cl := range []*client{gm, a} {
		var act protocol.SceneActivated
		if err := json.Unmarshal(cl.want(protocol.TypeSceneActivated).D, &act); err != nil {
			t.Fatal(err)
		}
		if ids := drawingIDs(act.Scene.Drawings); len(ids) != 1 || ids[0] != onCrypt {
			t.Errorf("scene.activated drawings %v, want [%s]", ids, onCrypt)
		}
	}
}

func drawingIDs(list []protocol.Drawing) []string {
	ids := make([]string, len(list))
	for i, d := range list {
		ids[i] = d.ID
	}
	return ids
}

func TestDeletingASceneDeletesItsDrawings(t *testing.T) {
	tb := newTable(t)
	tb.scene("Tavern")
	crypt := tb.scene("Crypt")
	_, _, gm := tb.trio()
	gm.send("drawing.create", "c-1", drawCmd(crypt, "rect", testRect))
	gm.want(protocol.TypeDrawingChanged)
	gm.wantAck("c-1")
	gm.send("scene.delete", "c-2", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	gm.want(protocol.TypeScenesChanged)
	gm.wantAck("c-2")

	var n int
	if err := tb.svc.db.QueryRow(`SELECT COUNT(*) FROM drawing`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d drawings left, err %v", n, err)
	}
}

func TestPingIsTransient(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()

	a.send("ping.place", "", fmt.Sprintf(`{"sceneId":%q,"x":640,"y":480.5}`, scene))
	for _, c := range []*client{a, b, gm} {
		env := c.want(protocol.TypePingShown)
		var d protocol.PingShown
		if err := json.Unmarshal(env.D, &d); err != nil {
			t.Fatal(err)
		}
		if env.Seq != 0 || env.ID != "" || d.PlayerID != tb.pid(tb.player) || d.SceneID != scene || d.X != 640 || d.Y != 480.5 || d.Focus {
			t.Errorf("got %+v %s", env, env.D)
		}
	}

	// Only the GM moves the cameras. With an id the player is told, without
	// one nothing happens.
	a.send("ping.place", "c-1", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":2,"focus":true}`, scene))
	a.wantErr("c-1", protocol.CodeForbidden)
	a.send("ping.place", "", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":2,"focus":true}`, scene))
	a.quiet()
	b.quiet()
	gm.quiet()

	gm.send("ping.place", "", fmt.Sprintf(`{"sceneId":%q,"x":10,"y":20,"focus":true}`, scene))
	for _, c := range []*client{a, b, gm} {
		var d protocol.PingShown
		if err := json.Unmarshal(c.want(protocol.TypePingShown).D, &d); err != nil {
			t.Fatal(err)
		}
		if !d.Focus || d.PlayerID != tb.pid(tb.gm) {
			t.Errorf("got %+v, want a focused ping of the GM", d)
		}
	}

	// A ping that does not make sense is dropped.
	a.send("ping.place", "", fmt.Sprintf(`{"sceneId":%q,"x":1e300,"y":2}`, scene))
	a.send("ping.place", "", `{"sceneId":"nope","x":1,"y":2}`)
	a.send("ping.place", "", `{"sceneId":"x","x":1}`)
	a.quiet()
	b.quiet()
	gm.quiet()
}

func TestRulerIsTransient(t *testing.T) {
	tb := newTable(t)
	scene := tb.scene("Tavern")
	a, b, gm := tb.trio()

	a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[[100,100],[250.5,180]]}`, scene))
	for _, c := range []*client{a, b, gm} {
		env := c.want(protocol.TypeRulerShown)
		var d protocol.RulerShown
		if err := json.Unmarshal(env.D, &d); err != nil {
			t.Fatal(err)
		}
		if env.Seq != 0 || d.PlayerID != tb.pid(tb.player) || len(d.Points) != 2 || d.Points[1] != [2]float64{250.5, 180} {
			t.Errorf("got %+v %s", env, env.D)
		}
	}

	// 51 points, none, or a point out of range: nothing is sent.
	pts := strings.TrimSuffix(strings.Repeat("[1,1],", 51), ",")
	a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[%s]}`, scene, pts))
	a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[]}`, scene))
	a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[[1e300,1]]}`, scene))
	a.send("ruler.update", "c-1", fmt.Sprintf(`{"sceneId":%q,"points":[%s]}`, scene, pts))
	a.wantErr("c-1", protocol.CodeBadRequest)
	b.quiet()
	gm.quiet()
	// 50 is fine.
	pts = strings.TrimSuffix(strings.Repeat("[1,1],", 50), ",")
	a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[%s]}`, scene, pts))
	a.want(protocol.TypeRulerShown)
	b.want(protocol.TypeRulerShown)
	gm.want(protocol.TypeRulerShown)

	// Clearing sends an empty array, not null.
	a.send("ruler.clear", "", fmt.Sprintf(`{"sceneId":%q}`, scene))
	for _, c := range []*client{a, b, gm} {
		env := c.want(protocol.TypeRulerShown)
		if !strings.Contains(string(env.D), `"points":[]`) {
			t.Errorf("got %s, want points as an empty array", env.D)
		}
	}
}

func TestRulerUpdatesAreLimitedTo30PerSecond(t *testing.T) {
	tb := newTable(t)
	frozen := time.Now()
	tb.now = func() time.Time { return frozen }
	scene := tb.scene("Tavern")
	a, b, _ := tb.trio()

	for range 60 {
		a.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[[1,1]]}`, scene))
	}
	if got := len(a.drainUntilPong()); got != 30 {
		t.Errorf("the author saw %d of 60 rulers, want 30", got)
	}
	if got := len(b.drainUntilPong()); got != 30 {
		t.Errorf("another player saw %d of 60 rulers, want 30", got)
	}
}

func TestPingAndRulerOfTheGMOnAnInactiveSceneStayWithTheGM(t *testing.T) {
	tb := newTable(t)
	tavern := tb.scene("Tavern") // active
	crypt := tb.scene("Crypt")
	a, b, gm := tb.trio()

	// A player has no business there.
	a.send("ping.place", "c-1", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":2}`, crypt))
	a.wantErr("c-1", protocol.CodeForbidden)
	a.send("ruler.update", "c-2", fmt.Sprintf(`{"sceneId":%q,"points":[[1,1]]}`, crypt))
	a.wantErr("c-2", protocol.CodeForbidden)
	a.send("ruler.clear", "c-3", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	a.wantErr("c-3", protocol.CodeForbidden)

	last := tb.seq()
	gm.send("ping.place", "", fmt.Sprintf(`{"sceneId":%q,"x":1,"y":2}`, crypt))
	gm.want(protocol.TypePingShown)
	gm.send("ruler.update", "", fmt.Sprintf(`{"sceneId":%q,"points":[[1,1],[2,2]]}`, crypt))
	gm.want(protocol.TypeRulerShown)
	gm.send("ruler.clear", "", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	gm.want(protocol.TypeRulerShown)
	a.quiet()
	b.quiet()

	// A player who comes back is sent no pings and rulers, only the events with a seq.
	gm.send("drawing.create", "c-4", drawCmd(tavern, "rect", testRect))
	ev := gm.recv()
	gm.wantAck("c-4")
	a.want(protocol.TypeDrawingChanged)
	b.want(protocol.TypeDrawingChanged)

	p2 := &client{t: t, c: tb.dial(tb.boris)}
	send(t, p2.c, fmt.Sprintf(`{"t":"sync.hello","id":"h-1","d":{"gameSessionId":%q,"lastSeq":%d}}`, tb.gs.ID, last))
	if ack := p2.wantAck("h-1"); !strings.Contains(string(ack.D), `"resumed":true`) {
		t.Fatalf("got %s, want a resume", ack.D)
	}
	if got := p2.drainUntilPong(); len(got) != 1 || got[0].T != protocol.TypeDrawingChanged || got[0].Seq != ev.Seq {
		t.Errorf("a resumed player got %+v, want only the drawing", got)
	}
}

func TestDrawingNeedsAPlayer(t *testing.T) {
	// The wire tests run as participants; this is the check of the handlers for
	// an account that has no player.
	if err := needPlayer(ws.Command{}); err == nil {
		t.Fatal("a caller without a player passed")
	}
	if err := needPlayer(ws.Command{Viewer: ws.Viewer{PlayerID: "01J"}}); err != nil {
		t.Fatal(err)
	}
}

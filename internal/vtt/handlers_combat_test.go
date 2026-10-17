package vtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// combatFrame decodes a combat.changed frame.
func combatFrame(t *testing.T, env protocol.Envelope) protocol.CombatChanged {
	t.Helper()
	if env.T != protocol.TypeCombatChanged {
		t.Fatalf("got %+v %s, want combat.changed", env, env.D)
	}
	var d protocol.CombatChanged
	if err := json.Unmarshal(env.D, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// fightTable is a table with an active scene, three tokens (a hidden one, one of
// Anna and one of Boris) and the three connections.
type fightTable struct {
	*table
	a, b, gm   *client
	scene      string
	hiddenTok  string
	annaTok    string
	borisTok   string
	nextCmdSeq int
}

func newFightTable(t *testing.T) *fightTable {
	t.Helper()
	tb := newTable(t)
	ft := &fightTable{table: tb, scene: tb.scene("Arena")}
	ctx := context.Background()
	mk := func(f protocol.TokenFields) string {
		res, err := tb.svc.CreateToken(ctx, tb.camp.ID, protocol.TokenCreate{SceneID: ft.scene, TokenFields: f})
		if err != nil {
			t.Fatal(err)
		}
		return res.Token.ID
	}
	x, y := fp(100), fp(100)
	ft.hiddenTok = mk(protocol.TokenFields{X: x, Y: y, Name: strp("ZZSECRET"), Hidden: bp(true)})
	ft.annaTok = mk(protocol.TokenFields{X: x, Y: y, Name: strp("Anna's knight"), Owners: []string{tb.pid(tb.player)}})
	ft.borisTok = mk(protocol.TokenFields{X: x, Y: y, Name: strp("Boris's rogue"), Owners: []string{tb.pid(tb.boris)}})
	ft.a, ft.b, ft.gm = tb.trio()
	return ft
}

func (ft *fightTable) cmd() string {
	ft.nextCmdSeq++
	return fmt.Sprintf("k-%d", ft.nextCmdSeq)
}

// byGM sends a command as the GM and reads the three combat.changed frames and the ack.
func (ft *fightTable) byGM(typ, data string) (gm, a, b protocol.CombatChanged) {
	ft.t.Helper()
	id := ft.cmd()
	ft.gm.send(typ, id, data)
	gm = combatFrame(ft.t, ft.gm.recv())
	ft.gm.wantAck(id)
	return gm, combatFrame(ft.t, ft.a.recv()), combatFrame(ft.t, ft.b.recv())
}

// byPlayer sends a command as cl and reads the three combat.changed frames; the
// ack follows the frame of the sender.
func (ft *fightTable) byPlayer(cl *client, typ, data string) (gm, a, b protocol.CombatChanged) {
	ft.t.Helper()
	id := ft.cmd()
	cl.send(typ, id, data)
	gm = combatFrame(ft.t, ft.gm.recv())
	a = combatFrame(ft.t, ft.a.recv())
	b = combatFrame(ft.t, ft.b.recv())
	cl.wantAck(id)
	return gm, a, b
}

func (ft *fightTable) sceneData(extra string) string {
	return fmt.Sprintf(`{"sceneId":%q%s}`, ft.scene, extra)
}

func combatantOf(c *protocol.Combat, tokenID string) *protocol.Combatant {
	for i := range c.Combatants {
		if c.Combatants[i].TokenID == tokenID {
			return &c.Combatants[i]
		}
	}
	return nil
}

func TestInitiativeTrackerOverTheWire(t *testing.T) {
	ft := newFightTable(t)
	a, b, gm := ft.a, ft.b, ft.gm

	// 1. start: everybody gets an empty fight in round 1.
	g, pa, pb := ft.byGM("combat.start", ft.sceneData(""))
	for who, d := range map[string]protocol.CombatChanged{"gm": g, "a": pa, "b": pb} {
		if d.SceneID != ft.scene || d.Combat == nil || d.Combat.Round != 1 || d.Combat.TurnCombatantID != nil ||
			d.Combat.Combatants == nil || len(d.Combat.Combatants) != 0 {
			t.Errorf("%s got %+v after the start", who, d.Combat)
		}
	}
	gm.send("combat.start", "dup", ft.sceneData(""))
	gm.wantErr("dup", protocol.CodeConflict)

	// 2. add: the GM sees three, the players two; the hidden one is marked.
	g, pa, pb = ft.byGM("combat.add", ft.sceneData(fmt.Sprintf(`,"tokenIds":[%q,%q,%q]`, ft.hiddenTok, ft.annaTok, ft.borisTok)))
	if len(g.Combat.Combatants) != 3 || len(pa.Combat.Combatants) != 2 || len(pb.Combat.Combatants) != 2 {
		t.Fatalf("members: gm %d, a %d, b %d; want 3, 2, 2", len(g.Combat.Combatants), len(pa.Combat.Combatants), len(pb.Combat.Combatants))
	}
	secret := combatantOf(g.Combat, ft.hiddenTok)
	if secret == nil || !secret.Hidden || secret.Name != "ZZSECRET" {
		t.Fatalf("hidden combatant for the GM = %+v", secret)
	}
	if combatantOf(pb.Combat, ft.hiddenTok) != nil {
		t.Fatal("a player got the hidden combatant")
	}
	annaID, borisID := combatantOf(g.Combat, ft.annaTok).ID, combatantOf(g.Combat, ft.borisTok).ID

	// 3. The hidden combatant is the fastest, so the first turn is theirs: players see no turn.
	ft.byGM("combat.setInitiative", fmt.Sprintf(`{"combatantId":%q,"value":20}`, secret.ID))
	ft.byGM("combat.setInitiative", fmt.Sprintf(`{"combatantId":%q,"value":5}`, borisID))
	g, pa, pb = ft.byGM("combat.next", ft.sceneData(""))
	if g.Combat.TurnCombatantID == nil || *g.Combat.TurnCombatantID != secret.ID {
		t.Errorf("the GM's turn = %v, want the hidden combatant", g.Combat.TurnCombatantID)
	}
	if pa.Combat.TurnCombatantID != nil || pb.Combat.TurnCombatantID != nil {
		t.Errorf("players see the turn of a hidden combatant: %v %v", pa.Combat.TurnCombatantID, pb.Combat.TurnCombatantID)
	}

	// 4. Anna rates her token; Boris may not rate it; Anna's roll comes from the roller.
	_, pa, _ = ft.byPlayer(a, "combat.setInitiative", fmt.Sprintf(`{"combatantId":%q,"value":11}`, annaID))
	if c := combatantOf(pa.Combat, ft.annaTok); c == nil || c.Initiative == nil || *c.Initiative != 11 {
		t.Errorf("Anna's initiative = %+v", c)
	}
	b.send("combat.setInitiative", "b-1", fmt.Sprintf(`{"combatantId":%q,"value":15}`, annaID))
	b.wantErr("b-1", protocol.CodeForbidden)
	ft.roller.set(13)
	_, pa, _ = ft.byPlayer(a, "combat.rollInitiative", fmt.Sprintf(`{"combatantIds":[%q]}`, annaID))
	if c := combatantOf(pa.Combat, ft.annaTok); c == nil || c.Initiative == nil || *c.Initiative != 13 {
		t.Errorf("Anna's rolled initiative = %+v, want 13", c)
	}
	a.send("combat.rollInitiative", "a-1", fmt.Sprintf(`{"combatantIds":[%q,%q]}`, annaID, borisID))
	a.wantErr("a-1", protocol.CodeForbidden)
	a.send("combat.setInitiative", "a-2", fmt.Sprintf(`{"combatantId":%q}`, annaID))
	a.wantErr("a-2", protocol.CodeBadRequest) // value is required
	a.send("combat.setInitiative", "a-3", fmt.Sprintf(`{"combatantId":%q,"value":100}`, annaID))
	a.wantErr("a-3", protocol.CodeBadRequest)
	a.send("combat.setInitiative", "a-4", fmt.Sprintf(`{"combatantId":%q,"value":null}`, annaID))
	a.want(protocol.TypeCombatChanged)
	b.want(protocol.TypeCombatChanged)
	gm.want(protocol.TypeCombatChanged)
	a.wantAck("a-4")
	ft.roller.set(13)
	ft.byPlayer(a, "combat.rollInitiative", fmt.Sprintf(`{"combatantIds":[%q]}`, annaID))

	// 5. Turns: GM from the hidden one to Anna; Boris may not end Anna's turn, Anna may.
	g, _, _ = ft.byGM("combat.next", ft.sceneData(""))
	if g.Combat.TurnCombatantID == nil || *g.Combat.TurnCombatantID != annaID {
		t.Fatalf("turn = %v, want Anna's", g.Combat.TurnCombatantID)
	}
	b.send("combat.next", "b-2", ft.sceneData(""))
	b.wantErr("b-2", protocol.CodeForbidden)
	a.send("combat.prev", "a-5", ft.sceneData(""))
	a.wantErr("a-5", protocol.CodeForbidden)
	g, pa, _ = ft.byPlayer(a, "combat.next", ft.sceneData(""))
	if g.Combat.TurnCombatantID == nil || *g.Combat.TurnCombatantID != borisID || pa.Combat.TurnCombatantID == nil {
		t.Errorf("after Anna's next: gm %v, a %v; want Boris's turn", g.Combat.TurnCombatantID, pa.Combat.TurnCombatantID)
	}
	a.send("combat.next", "a-6", ft.sceneData(""))
	a.wantErr("a-6", protocol.CodeForbidden)

	// 6. A fight on an inactive scene is the GM's business.
	crypt := ft.table.scene("Crypt")
	gm.send("combat.start", "k-crypt", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	if d := combatFrame(t, gm.recv()); d.SceneID != crypt || d.Combat == nil {
		t.Errorf("GM got %+v for the inactive scene", d)
	}
	gm.wantAck("k-crypt")
	a.wantNoop(ft.noopSeq(a))
	b.wantNoop(ft.noopSeq(b))
	a.send("combat.next", "a-7", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	a.wantErr("a-7", protocol.CodeForbidden)
	a.send("combat.setInitiative", "a-8", `{"combatantId":"x","value":1}`)
	a.wantErr("a-8", protocol.CodeNotFound)

	// 7. A snapshot of Boris holds the fight without the hidden one.
	boris := ws.Viewer{PlayerID: ft.pid(ft.boris)}
	if c := ft.snapshotCombat(boris); c == nil || len(c.Combatants) != 2 || c.Round != 1 || c.TurnCombatantID == nil || *c.TurnCombatantID != borisID {
		t.Fatalf("Boris's snapshot combat = %+v", c)
	}
	if c := ft.snapshotCombat(ws.Viewer{IsGM: true}); c == nil || len(c.Combatants) != 3 {
		t.Fatalf("the GM's snapshot combat = %+v", c)
	}

	// 8. Deleting the token that holds the turn takes it out of the fight and clears the turn.
	gm.send("token.delete", "k-del", fmt.Sprintf(`{"tokenId":%q}`, ft.borisTok))
	gm.want(protocol.TypeTokenRemoved)
	if d := combatFrame(t, gm.recv()); d.Combat.TurnCombatantID != nil || len(d.Combat.Combatants) != 2 || combatantOf(d.Combat, ft.borisTok) != nil {
		t.Errorf("GM after the token deletion = %+v", d.Combat)
	}
	gm.wantAck("k-del")
	for _, cl := range []*client{a, b} {
		cl.want(protocol.TypeTokenRemoved)
		if d := combatFrame(t, cl.recv()); d.Combat.TurnCombatantID != nil || len(d.Combat.Combatants) != 1 {
			t.Errorf("player after the token deletion = %+v", d.Combat)
		}
	}

	// 9. End: everybody gets null, and snapshots have none.
	g, pa, pb = ft.byGM("combat.end", ft.sceneData(""))
	if g.Combat != nil || pa.Combat != nil || pb.Combat != nil || g.SceneID != ft.scene {
		t.Errorf("after the end: %+v %+v %+v", g.Combat, pa.Combat, pb.Combat)
	}
	if c := ft.snapshotCombat(boris); c != nil {
		t.Errorf("snapshot after the end has %+v", c)
	}
	gm.send("combat.end", "k-end2", ft.sceneData(""))
	gm.wantErr("k-end2", protocol.CodeNotFound)
	gm.send("combat.next", "k-next2", ft.sceneData(""))
	gm.wantErr("k-next2", protocol.CodeNotFound)

	// The hidden combatant never reached the players, neither by id nor by name.
	for who, cl := range map[string]*client{"a": a, "b": b} {
		for _, frame := range cl.frames {
			for _, leak := range []string{secret.ID, "ZZSECRET", ft.hiddenTok} {
				if strings.Contains(frame, leak) {
					t.Errorf("%s got %q in %s", who, leak, frame)
				}
			}
		}
	}
}

// noopSeq is the seq of the next frame cl is going to get: the one after its last.
func (ft *fightTable) noopSeq(cl *client) int64 {
	ft.t.Helper()
	var env protocol.Envelope
	if n := len(cl.frames); n > 0 {
		for i := n - 1; i >= 0; i-- {
			if err := json.Unmarshal([]byte(cl.frames[i]), &env); err == nil && env.Seq > 0 {
				break
			}
		}
	}
	return env.Seq + 1
}

func TestActivatingASceneSendsItsFight(t *testing.T) {
	ft := newFightTable(t)
	ft.byGM("combat.start", ft.sceneData(""))
	ft.byGM("combat.add", ft.sceneData(fmt.Sprintf(`,"tokenIds":[%q,%q,%q]`, ft.hiddenTok, ft.annaTok, ft.borisTok)))

	// Move the table to another scene: its fight is not on the table, so only
	// scene.activated follows.
	crypt := ft.table.scene("Crypt")
	ft.gm.send("scene.activate", "k-crypt", fmt.Sprintf(`{"sceneId":%q}`, crypt))
	for _, cl := range []*client{ft.gm, ft.a, ft.b} {
		cl.want(protocol.TypeSceneActivated)
	}
	ft.gm.wantAck("k-crypt")

	// Back to the scene with the fight: scene.activated, then combat.changed.
	ft.gm.send("scene.activate", "k-arena", ft.sceneData(""))
	var frames []protocol.CombatChanged
	for _, cl := range []*client{ft.gm, ft.a, ft.b} {
		cl.want(protocol.TypeSceneActivated)
		frames = append(frames, combatFrame(t, cl.recv()))
	}
	ft.gm.wantAck("k-arena")
	for i, who := range []string{"gm", "a", "b"} {
		d := frames[i]
		want := 2
		if i == 0 {
			want = 3
		}
		if d.SceneID != ft.scene || d.Combat == nil || len(d.Combat.Combatants) != want {
			t.Errorf("%s got %+v after the activation, want %d combatants", who, d.Combat, want)
		}
	}
	for who, cl := range map[string]*client{"a": ft.a, "b": ft.b} {
		for _, frame := range cl.frames {
			if strings.Contains(frame, "ZZSECRET") || strings.Contains(frame, ft.hiddenTok) {
				t.Errorf("%s got a hidden combatant in %s", who, frame)
			}
		}
	}
}

func TestCombatCommandsAreForTheGMExceptThree(t *testing.T) {
	ft := newFightTable(t)
	for _, c := range []struct{ typ, data string }{
		{"combat.start", ft.sceneData("")},
		{"combat.end", ft.sceneData("")},
		{"combat.add", ft.sceneData(`,"tokenIds":["x"]`)},
		{"combat.remove", `{"combatantId":"x"}`},
		{"combat.setHidden", `{"combatantId":"x","hidden":true}`},
		{"combat.prev", ft.sceneData("")},
	} {
		ft.a.send(c.typ, "p-"+c.typ, c.data)
		ft.a.wantErr("p-"+c.typ, protocol.CodeForbidden)
	}
	ft.gm.quiet()
}

func TestCombatCommandErrors(t *testing.T) {
	ft := newFightTable(t)
	gm := ft.gm
	for _, c := range []struct{ name, typ, data, code string }{
		{"next without a fight", "combat.next", ft.sceneData(""), protocol.CodeNotFound},
		{"add without a fight", "combat.add", ft.sceneData(fmt.Sprintf(`,"tokenIds":[%q]`, ft.annaTok)), protocol.CodeNotFound},
		{"start on an unknown scene", "combat.start", `{"sceneId":"nope"}`, protocol.CodeNotFound},
		{"start with an extra field", "combat.start", ft.sceneData(`,"x":1`), protocol.CodeBadRequest},
		{"remove an unknown combatant", "combat.remove", `{"combatantId":"nope"}`, protocol.CodeNotFound},
		{"hide an unknown combatant", "combat.setHidden", `{"combatantId":"nope","hidden":true}`, protocol.CodeNotFound},
		{"roll an unknown combatant", "combat.rollInitiative", `{"combatantIds":["nope"]}`, protocol.CodeNotFound},
		{"roll nobody", "combat.rollInitiative", `{"combatantIds":[]}`, protocol.CodeBadRequest},
		{"fractional initiative", "combat.setInitiative", `{"combatantId":"x","value":1.5}`, protocol.CodeBadRequest},
		{"missing initiative", "combat.setInitiative", `{"combatantId":"x"}`, protocol.CodeBadRequest},
	} {
		gm.send(c.typ, "e", c.data)
		env := gm.recv()
		if env.T != protocol.TypeError {
			t.Errorf("%s: got %+v", c.name, env)
			continue
		}
		var d protocol.ErrorData
		if err := json.Unmarshal(env.D, &d); err != nil || d.Code != c.code {
			t.Errorf("%s: got %s, want %s", c.name, env.D, c.code)
		}
	}

	ft.byGM("combat.start", ft.sceneData(""))
	gm.send("combat.next", "e-1", ft.sceneData(""))
	gm.wantErr("e-1", protocol.CodeConflict) // empty fight
	gm.send("combat.add", "e-2", ft.sceneData(`,"tokenIds":["nope"]`))
	gm.wantErr("e-2", protocol.CodeNotFound)
	gm.send("combat.add", "e-3", ft.sceneData(`,"tokenIds":[]`))
	gm.wantErr("e-3", protocol.CodeBadRequest)
	_, _, _ = ft.byGM("combat.add", ft.sceneData(fmt.Sprintf(`,"tokenIds":[%q]`, ft.annaTok)))
	gm.send("combat.prev", "e-4", ft.sceneData(""))
	gm.wantErr("e-4", protocol.CodeConflict) // no turn yet
}

func (ft *fightTable) snapshotCombat(v ws.Viewer) *protocol.Combat {
	ft.t.Helper()
	var tbl protocol.SnapshotTable
	if err := ft.svc.FillSnapshot(context.Background(), v, ft.camp.ID, &tbl); err != nil {
		ft.t.Fatal(err)
	}
	return tbl.Combat
}

// A new connection gets the fight in its snapshot, without the hidden combatant.
func TestSnapshotOnTheWireCarriesTheFight(t *testing.T) {
	ft := newFightTable(t)
	ctx := context.Background()
	if _, err := ft.svc.StartCombat(ctx, ft.camp.ID, ft.scene); err != nil {
		t.Fatal(err)
	}
	if _, err := ft.svc.AddCombatants(ctx, ft.camp.ID, protocol.CombatAdd{SceneID: ft.scene, TokenIDs: []string{ft.hiddenTok, ft.annaTok}}); err != nil {
		t.Fatal(err)
	}
	ft.gm.quiet()
	cl := ft.connect(ft.boris)
	snap := snapshotOf(t, protocol.Envelope{D: json.RawMessage(strings.TrimSpace(frameData(t, cl.frames[0])))})
	if snap.Game == nil || snap.Game.Combat == nil || len(snap.Game.Combat.Combatants) != 1 || snap.Game.Combat.Combatants[0].TokenID != ft.annaTok {
		t.Fatalf("snapshot combat = %+v", snap.Game.Combat)
	}
	if strings.Contains(cl.frames[0], "ZZSECRET") || strings.Contains(cl.frames[0], ft.hiddenTok) {
		t.Errorf("the snapshot leaks the hidden combatant: %s", cl.frames[0])
	}
}

func frameData(t *testing.T, frame string) string {
	t.Helper()
	var env protocol.Envelope
	if err := json.Unmarshal([]byte(frame), &env); err != nil {
		t.Fatal(err)
	}
	return string(env.D)
}

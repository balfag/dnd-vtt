package vtt

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// sequenceRoller answers with the given values in turn, whatever the sides are.
type sequenceRoller struct {
	mu     sync.Mutex
	values []int
}

func (r *sequenceRoller) Roll(int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.values) == 0 {
		return 1
	}
	n := r.values[0]
	r.values = r.values[1:]
	return n
}

func (r *sequenceRoller) set(values ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = values
}

func orderIDs(cs []combatantRow) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.id
	}
	return out
}

func memberIDs(cv *CombatView) []string { return orderIDs(cv.Members) }

func TestTurnOrder(t *testing.T) {
	// Added in this order; ids sort the other way round, so a tie that is
	// broken by id and not by added_seq is noticed.
	in := []combatantRow{
		{id: "d", initiative: intp(10), addedSeq: 1},
		{id: "c", initiative: nil, addedSeq: 2},
		{id: "b", initiative: intp(15), addedSeq: 3},
		{id: "a", initiative: intp(10), addedSeq: 4},
	}
	got := orderIDs(turnOrder(in))
	if want := []string{"b", "d", "a", "c"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if in[0].id != "d" || in[1].id != "c" {
		t.Error("turnOrder changed its argument")
	}

	negative := []combatantRow{
		{id: "x", initiative: intp(-3), addedSeq: 1},
		{id: "y", initiative: nil, addedSeq: 2},
		{id: "z", initiative: intp(0), addedSeq: 3},
		{id: "n1", initiative: nil, addedSeq: 0},
	}
	if got, want := orderIDs(turnOrder(negative)), []string{"z", "x", "n1", "y"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestNextAndPrev(t *testing.T) {
	order := []combatantRow{{id: "a"}, {id: "b"}, {id: "c"}}

	// next through two rounds.
	cur, round := "", 1
	steps := []struct {
		want  string
		round int
	}{{"a", 1}, {"b", 1}, {"c", 1}, {"a", 2}, {"b", 2}}
	for i, st := range steps {
		id, grows, err := next(order, cur)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if grows {
			round++
		}
		cur = id
		if cur != st.want || round != st.round {
			t.Errorf("step %d: turn %s round %d, want %s round %d", i, cur, round, st.want, st.round)
		}
	}

	// prev walks back to the very first turn and stops there.
	back := []struct {
		want  string
		round int
	}{{"a", 2}, {"c", 1}, {"b", 1}, {"a", 1}}
	for i, st := range back {
		id, shrinks, err := prev(order, cur, round)
		if err != nil {
			t.Fatalf("back %d: %v", i, err)
		}
		if shrinks {
			round--
		}
		cur = id
		if cur != st.want || round != st.round {
			t.Errorf("back %d: turn %s round %d, want %s round %d", i, cur, round, st.want, st.round)
		}
	}
	if _, _, err := prev(order, "a", 1); !errors.Is(err, ErrConflict) {
		t.Errorf("prev at the first turn of round 1: %v, want ErrConflict", err)
	}
	if _, _, err := prev(order, "", 1); !errors.Is(err, ErrConflict) {
		t.Errorf("prev before the first turn: %v, want ErrConflict", err)
	}
	if _, _, err := next(nil, ""); !errors.Is(err, ErrConflict) {
		t.Errorf("next of an empty fight: %v, want ErrConflict", err)
	}
	if id, grows, err := next([]combatantRow{{id: "solo"}}, "solo"); err != nil || id != "solo" || !grows {
		t.Errorf("next of a single combatant = %s %v %v, want solo, true", id, grows, err)
	}
}

// fight is a scene with a started fight and three tokens that take part.
type fight struct {
	e      *env
	ctx    context.Context
	scene  string
	tokens []protocol.Token
	gm     ws.Viewer
	res    CombatResult
}

func newFight(t *testing.T) *fight {
	t.Helper()
	e := newEnv(t)
	f := &fight{e: e, ctx: context.Background(), scene: e.create("Arena").SceneID, gm: ws.Viewer{IsGM: true}}
	for _, name := range []string{"Goblin", "Orc", "Wolf"} {
		f.tokens = append(f.tokens, e.token(f.scene, protocol.TokenFields{Name: strp(name)}))
	}
	if _, err := e.svc.StartCombat(f.ctx, e.camp, f.scene); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.AddCombatants(f.ctx, e.camp, protocol.CombatAdd{SceneID: f.scene, TokenIDs: []string{f.tokens[0].ID, f.tokens[1].ID, f.tokens[2].ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.res = res
	return f
}

// id returns the combatant of token i.
func (f *fight) id(i int) string {
	for _, m := range f.res.Combat.Members {
		if m.tokenID == f.tokens[i].ID {
			return m.id
		}
	}
	f.e.t.Fatalf("token %d is not in the fight", i)
	return ""
}

func (f *fight) rate(i, value int) {
	f.e.t.Helper()
	res, err := f.e.svc.SetInitiative(f.ctx, f.e.camp, f.gm, f.id(i), intp(value))
	if err != nil {
		f.e.t.Fatal(err)
	}
	f.res = res
}

func (f *fight) next() CombatResult {
	f.e.t.Helper()
	res, err := f.e.svc.NextTurn(f.ctx, f.e.camp, f.gm, f.scene)
	if err != nil {
		f.e.t.Fatal(err)
	}
	f.res = res
	return res
}

func TestStartAndEndCombat(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	scene := e.create("Arena").SceneID

	if _, err := e.svc.EndCombat(ctx, e.camp, scene); !errors.Is(err, ErrNotFound) {
		t.Errorf("end without a fight: %v, want ErrNotFound", err)
	}
	if _, err := e.svc.StartCombat(ctx, e.camp, "no-such-scene"); !errors.Is(err, ErrNotFound) {
		t.Errorf("start on an unknown scene: %v, want ErrNotFound", err)
	}
	res, err := e.svc.StartCombat(ctx, e.camp, scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Combat == nil || res.Combat.Round != 1 || res.Combat.Turn != "" || len(res.Combat.Members) != 0 || !res.Active {
		t.Errorf("started = %+v", res)
	}
	if _, err := e.svc.StartCombat(ctx, e.camp, scene); !errors.Is(err, ErrConflict) {
		t.Errorf("second start: %v, want ErrConflict", err)
	}
	if _, err := e.svc.StartCombat(ctx, e.other, scene); !errors.Is(err, ErrNotFound) {
		t.Errorf("start of another campaign's scene: %v, want ErrNotFound", err)
	}
	end, err := e.svc.EndCombat(ctx, e.camp, scene)
	if err != nil || end.Combat != nil || end.SceneID != scene {
		t.Errorf("end = %+v, %v", end, err)
	}
	if _, err := e.svc.StartCombat(ctx, e.camp, scene); err != nil {
		t.Errorf("start after the end: %v", err)
	}
}

func TestEndCombatTakesTheCombatantsWithIt(t *testing.T) {
	f := newFight(t)
	f.next()
	if _, err := f.e.svc.EndCombat(f.ctx, f.e.camp, f.scene); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.e.db.QueryRow(`SELECT (SELECT COUNT(*) FROM combat) + (SELECT COUNT(*) FROM combatant)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left after the end: %d, %v", n, err)
	}
}

func TestAddCombatants(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	scene := e.create("Arena").SceneID
	other := e.create("Elsewhere").SceneID
	plain := e.token(scene, protocol.TokenFields{Name: strp("Plain")})
	hidden := e.token(scene, protocol.TokenFields{Name: strp("Hidden"), Hidden: bp(true)})
	gmLayer := e.token(scene, protocol.TokenFields{Name: strp("GM layer"), Layer: strp("gm")})
	foreign := e.token(other, protocol.TokenFields{Name: strp("Foreign")})

	if _, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{plain.ID}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("add without a fight: %v, want ErrNotFound", err)
	}
	if _, err := e.svc.StartCombat(ctx, e.camp, scene); err != nil {
		t.Fatal(err)
	}
	for name, ids := range map[string][]string{
		"none":      {},
		"repeated":  {plain.ID, plain.ID},
		"too many":  make([]string, 51),
		"unique 51": nil,
	} {
		if name == "unique 51" {
			for i := range 51 {
				ids = append(ids, string(rune('A'+i/26))+string(rune('a'+i%26)))
			}
		}
		if _, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: ids}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{plain.ID, foreign.ID}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a token of another scene: %v, want ErrNotFound", err)
	}
	if _, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{plain.ID, "nope"}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown token: %v, want ErrNotFound", err)
	}
	// The failures above added nothing.
	res, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{plain.ID, hidden.ID}})
	if err != nil || len(res.Combat.Members) != 2 {
		t.Fatalf("add = %+v, %v", res, err)
	}
	// Already added tokens are skipped, the new ones follow.
	res, err = e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{hidden.ID, gmLayer.ID, plain.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Combat.Members) != 3 {
		t.Fatalf("members = %d, want 3", len(res.Combat.Members))
	}
	byToken := map[string]combatantRow{}
	for _, m := range res.Combat.Members {
		byToken[m.tokenID] = m
	}
	if byToken[plain.ID].hidden || !byToken[hidden.ID].hidden || !byToken[gmLayer.ID].hidden {
		t.Errorf("hidden flags: plain %v, hidden %v, gm layer %v", byToken[plain.ID].hidden, byToken[hidden.ID].hidden, byToken[gmLayer.ID].hidden)
	}
	if byToken[plain.ID].addedSeq != 1 || byToken[hidden.ID].addedSeq != 2 || byToken[gmLayer.ID].addedSeq != 3 {
		t.Errorf("added_seq: %d %d %d, want 1 2 3", byToken[plain.ID].addedSeq, byToken[hidden.ID].addedSeq, byToken[gmLayer.ID].addedSeq)
	}
}

func TestTurnStaysWithTheCombatantWhenTheOrderChanges(t *testing.T) {
	f := newFight(t)
	f.rate(0, 10)
	f.rate(1, 20)
	f.rate(2, 5)
	// Order: Orc 20, Goblin 10, Wolf 5.
	if got, want := memberIDs(f.res.Combat), []string{f.id(1), f.id(0), f.id(2)}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	f.next() // Orc
	f.next() // Goblin
	if f.res.Combat.Turn != f.id(0) {
		t.Fatalf("turn = %s, want the goblin", f.res.Combat.Turn)
	}
	// The wolf now outruns everyone: the goblin moves from the 2nd to the 3rd place.
	f.rate(2, 30)
	if got, want := memberIDs(f.res.Combat), []string{f.id(2), f.id(1), f.id(0)}; !slices.Equal(got, want) {
		t.Fatalf("order after the change = %v, want %v", got, want)
	}
	if f.res.Combat.Turn != f.id(0) {
		t.Errorf("the turn moved to %s with the order, want it to stay with the goblin", f.res.Combat.Turn)
	}
	f.next() // after the goblin, the last one: the wolf, round 2
	if f.res.Combat.Turn != f.id(2) || f.res.Combat.Round != 2 {
		t.Errorf("after next: turn %s round %d, want the wolf in round 2", f.res.Combat.Turn, f.res.Combat.Round)
	}
}

func TestNextAndPrevOverTheService(t *testing.T) {
	f := newFight(t)
	f.rate(0, 3)
	f.rate(1, 2)
	f.rate(2, 1)
	ctx, e := f.ctx, f.e

	if _, err := e.svc.PrevTurn(ctx, e.camp, f.gm, f.scene); !errors.Is(err, ErrConflict) {
		t.Errorf("prev before the first turn: %v, want ErrConflict", err)
	}
	f.next()
	if f.res.Combat.Turn != f.id(0) || f.res.Combat.Round != 1 {
		t.Errorf("first next: %s round %d", f.res.Combat.Turn, f.res.Combat.Round)
	}
	if _, err := e.svc.PrevTurn(ctx, e.camp, f.gm, f.scene); !errors.Is(err, ErrConflict) {
		t.Errorf("prev at the first turn of round 1: %v, want ErrConflict", err)
	}
	f.next()
	f.next()
	f.next() // round 2, the goblin again
	if f.res.Combat.Turn != f.id(0) || f.res.Combat.Round != 2 {
		t.Fatalf("after a full round: %s round %d", f.res.Combat.Turn, f.res.Combat.Round)
	}
	res, err := e.svc.PrevTurn(ctx, e.camp, f.gm, f.scene)
	if err != nil || res.Combat.Turn != f.id(2) || res.Combat.Round != 1 {
		t.Errorf("prev from round 2: %+v, %v, want the wolf in round 1", res.Combat, err)
	}
}

func TestNextOfAnEmptyFightIsAConflict(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Arena").SceneID
	if _, err := e.svc.StartCombat(context.Background(), e.camp, scene); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.NextTurn(context.Background(), e.camp, ws.Viewer{IsGM: true}, scene); !errors.Is(err, ErrConflict) {
		t.Errorf("next of an empty fight: %v, want ErrConflict", err)
	}
	if _, err := e.svc.NextTurn(context.Background(), e.camp, ws.Viewer{IsGM: true}, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("next without a fight: %v, want ErrNotFound", err)
	}
}

func TestRemoveCombatantPassesTheTurnFirst(t *testing.T) {
	t.Run("a combatant in the middle", func(t *testing.T) {
		f := newFight(t)
		f.rate(0, 3)
		f.rate(1, 2)
		f.rate(2, 1)
		f.next()
		f.next() // the orc
		res, err := f.e.svc.RemoveCombatant(f.ctx, f.e.camp, f.id(1))
		if err != nil {
			t.Fatal(err)
		}
		if res.Combat.Turn != f.id(2) || res.Combat.Round != 1 || len(res.Combat.Members) != 2 {
			t.Errorf("after removal: turn %s round %d, %d members; want the wolf, 1, 2", res.Combat.Turn, res.Combat.Round, len(res.Combat.Members))
		}
	})
	t.Run("the last in the order goes to the first, the round stays", func(t *testing.T) {
		f := newFight(t)
		f.rate(0, 3)
		f.rate(1, 2)
		f.rate(2, 1)
		f.next()
		f.next()
		f.next() // the wolf, last
		res, err := f.e.svc.RemoveCombatant(f.ctx, f.e.camp, f.id(2))
		if err != nil {
			t.Fatal(err)
		}
		if res.Combat.Turn != f.id(0) || res.Combat.Round != 1 {
			t.Errorf("after removal: turn %s round %d, want the goblin in round 1", res.Combat.Turn, res.Combat.Round)
		}
	})
	t.Run("the only one leaves no turn", func(t *testing.T) {
		e := newEnv(t)
		ctx := context.Background()
		scene := e.create("Arena").SceneID
		tok := e.token(scene, protocol.TokenFields{})
		if _, err := e.svc.StartCombat(ctx, e.camp, scene); err != nil {
			t.Fatal(err)
		}
		added, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{tok.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.NextTurn(ctx, e.camp, ws.Viewer{IsGM: true}, scene); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.RemoveCombatant(ctx, e.camp, added.Combat.Members[0].id)
		if err != nil {
			t.Fatal(err)
		}
		if res.Combat.Turn != "" || len(res.Combat.Members) != 0 || res.Combat.Round != 1 {
			t.Errorf("after removal: %+v", res.Combat)
		}
	})
	t.Run("someone else keeps the turn", func(t *testing.T) {
		f := newFight(t)
		f.rate(0, 3)
		f.rate(1, 2)
		f.rate(2, 1)
		f.next() // the goblin
		res, err := f.e.svc.RemoveCombatant(f.ctx, f.e.camp, f.id(2))
		if err != nil || res.Combat.Turn != f.id(0) {
			t.Errorf("removal of the wolf: turn %s, %v, want the goblin", res.Combat.Turn, err)
		}
	})
	t.Run("an unknown or foreign combatant", func(t *testing.T) {
		f := newFight(t)
		if _, err := f.e.svc.RemoveCombatant(f.ctx, f.e.camp, "nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown: %v, want ErrNotFound", err)
		}
		if _, err := f.e.svc.RemoveCombatant(f.ctx, f.e.other, f.id(0)); !errors.Is(err, ErrNotFound) {
			t.Errorf("foreign: %v, want ErrNotFound", err)
		}
	})
}

func TestRollInitiativeUsesTheRoller(t *testing.T) {
	f := newFight(t)
	r := &sequenceRoller{}
	r.set(7, 19)
	res, err := f.e.svc.RollInitiative(f.ctx, f.e.camp, f.gm, []string{f.id(0), f.id(1)}, r)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*int{}
	for _, m := range res.Combat.Members {
		got[m.id] = m.initiative
	}
	if got[f.id(0)] == nil || *got[f.id(0)] != 7 || got[f.id(1)] == nil || *got[f.id(1)] != 19 || got[f.id(2)] != nil {
		t.Errorf("initiatives = %v, want 7, 19 and unset", got)
	}
	if order := memberIDs(res.Combat); !slices.Equal(order, []string{f.id(1), f.id(0), f.id(2)}) {
		t.Errorf("order = %v", order)
	}

	for name, ids := range map[string][]string{"none": {}, "repeated": {f.id(0), f.id(0)}} {
		if _, err := f.e.svc.RollInitiative(f.ctx, f.e.camp, f.gm, ids, r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, err := f.e.svc.RollInitiative(f.ctx, f.e.camp, f.gm, []string{f.id(0), "nope"}, r); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v, want ErrNotFound", err)
	}
}

func TestSetInitiativeLimits(t *testing.T) {
	f := newFight(t)
	set := func(v *int) (CombatResult, error) {
		return f.e.svc.SetInitiative(f.ctx, f.e.camp, f.gm, f.id(0), v)
	}
	for _, ok := range []int{-99, 0, 99} {
		res, err := set(intp(ok))
		if err != nil {
			t.Errorf("%d: %v", ok, err)
			continue
		}
		for _, m := range res.Combat.Members {
			if m.id == f.id(0) && (m.initiative == nil || *m.initiative != ok) {
				t.Errorf("stored %v, want %d", m.initiative, ok)
			}
		}
	}
	for _, bad := range []int{-100, 100} {
		if _, err := set(intp(bad)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%d: %v, want ErrInvalid", bad, err)
		}
	}
	res, err := set(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Combat.Members {
		if m.initiative != nil {
			t.Errorf("initiative %d left after null", *m.initiative)
		}
	}
}

func TestSetCombatantHidden(t *testing.T) {
	f := newFight(t)
	res, err := f.e.svc.SetCombatantHidden(f.ctx, f.e.camp, f.id(0), true)
	if err != nil {
		t.Fatal(err)
	}
	hidden := 0
	for _, m := range res.Combat.Members {
		if m.hidden {
			hidden++
		}
	}
	if hidden != 1 {
		t.Errorf("%d hidden, want 1", hidden)
	}
	if _, err := f.e.svc.SetCombatantHidden(f.ctx, f.e.camp, "nope", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v, want ErrNotFound", err)
	}
}

func TestPlayerRightsInTheService(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	anna, boris := e.player(e.camp, "anna", false), e.player(e.camp, "boris", false)
	scene := e.create("Arena").SceneID // active: the first one
	idle := e.create("Idle").SceneID
	mine := e.token(scene, protocol.TokenFields{Name: strp("Mine"), Owners: []string{anna}})
	theirs := e.token(scene, protocol.TokenFields{Name: strp("Theirs"), Owners: []string{boris}})
	idleTok := e.token(idle, protocol.TokenFields{Name: strp("Idle"), Owners: []string{anna}})
	for _, sc := range []string{scene, idle} {
		if _, err := e.svc.StartCombat(ctx, e.camp, sc); err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: scene, TokenIDs: []string{mine.ID, theirs.ID}})
	if err != nil {
		t.Fatal(err)
	}
	idleRes, err := e.svc.AddCombatants(ctx, e.camp, protocol.CombatAdd{SceneID: idle, TokenIDs: []string{idleTok.ID}})
	if err != nil {
		t.Fatal(err)
	}
	mineID, theirsID, idleID := res.Combat.Members[0].id, res.Combat.Members[1].id, idleRes.Combat.Members[0].id
	a, gm := ws.Viewer{PlayerID: anna}, ws.Viewer{IsGM: true}

	forbidden := func(what string, err error) {
		t.Helper()
		var ce *ws.CommandError
		if !errors.As(err, &ce) || ce.Code != protocol.CodeForbidden {
			t.Errorf("%s: %v, want forbidden", what, err)
		}
	}
	_, err = e.svc.SetInitiative(ctx, e.camp, a, mineID, intp(12))
	if err != nil {
		t.Errorf("own set: %v", err)
	}
	_, err = e.svc.SetInitiative(ctx, e.camp, a, theirsID, intp(12))
	forbidden("set of another's", err)
	_, err = e.svc.SetInitiative(ctx, e.camp, a, idleID, intp(12))
	forbidden("set on an inactive scene", err)
	_, err = e.svc.RollInitiative(ctx, e.camp, a, []string{mineID}, &sequenceRoller{values: []int{9}})
	if err != nil {
		t.Errorf("own roll: %v", err)
	}
	r := &sequenceRoller{values: []int{4, 5}}
	_, err = e.svc.RollInitiative(ctx, e.camp, a, []string{mineID, theirsID}, r)
	forbidden("roll with another's among them", err)
	if len(r.values) != 2 {
		t.Error("the roller was used though the command was refused")
	}

	// The turn: only its owner (or the GM) passes it on.
	_, err = e.svc.NextTurn(ctx, e.camp, a, scene)
	forbidden("next before any turn", err)
	if _, err := e.svc.SetInitiative(ctx, e.camp, gm, theirsID, intp(20)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.NextTurn(ctx, e.camp, gm, scene); err != nil { // Boris's combatant
		t.Fatal(err)
	}
	_, err = e.svc.NextTurn(ctx, e.camp, a, scene)
	forbidden("next in another's turn", err)
	if _, err := e.svc.NextTurn(ctx, e.camp, ws.Viewer{PlayerID: boris}, scene); err != nil {
		t.Errorf("next by the owner: %v", err)
	}
	_, err = e.svc.NextTurn(ctx, e.camp, a, idle)
	forbidden("next on an inactive scene", err)
}

func TestDeletingATokenOrASceneDeletesTheCombatant(t *testing.T) {
	f := newFight(t)
	f.rate(0, 3)
	f.rate(1, 2)
	f.next() // the goblin holds the turn
	res, err := f.e.svc.DeleteToken(f.ctx, f.e.camp, f.tokens[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Combat == nil || res.Combat.Combat.Turn != "" || len(res.Combat.Combat.Members) != 2 {
		t.Errorf("after the token went: %+v", res.Combat)
	}
	if res.Combat.SceneID != f.scene || !res.Combat.Active {
		t.Errorf("result scene %s active %v", res.Combat.SceneID, res.Combat.Active)
	}

	// A token outside any fight carries no fight in the result.
	other := f.e.create("Other").SceneID
	lone := f.e.token(other, protocol.TokenFields{})
	res, err = f.e.svc.DeleteToken(f.ctx, f.e.camp, lone.ID)
	if err != nil || res.Combat != nil {
		t.Errorf("token without a fight: %+v, %v", res.Combat, err)
	}

	// Deleting a scene deletes its fight (the active scene cannot be deleted).
	inactive := f.e.create("Doomed").SceneID
	tok := f.e.token(inactive, protocol.TokenFields{})
	if _, err := f.e.svc.StartCombat(f.ctx, f.e.camp, inactive); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.svc.AddCombatants(f.ctx, f.e.camp, protocol.CombatAdd{SceneID: inactive, TokenIDs: []string{tok.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.svc.DeleteScene(f.ctx, f.e.camp, inactive); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.e.db.QueryRow(`SELECT COUNT(*) FROM combat WHERE scene_id = ?`, inactive).Scan(&n); err != nil || n != 0 {
		t.Errorf("combat rows of the deleted scene: %d, %v", n, err)
	}
	if err := f.e.db.QueryRow(`SELECT COUNT(*) FROM combatant WHERE scene_id = ?`, inactive).Scan(&n); err != nil || n != 0 {
		t.Errorf("combatant rows of the deleted scene: %d, %v", n, err)
	}
}

func TestRenderForTheViewer(t *testing.T) {
	cv := CombatView{
		SceneID: "s", Round: 3, Turn: "h",
		Members: []combatantRow{
			{id: "v", tokenID: "tv", name: "Visible", nameVisibility: "all", initiative: intp(9)},
			{id: "h", tokenID: "th", name: "Secret", nameVisibility: "all", hidden: true},
			{id: "n", tokenID: "tn", name: "Nameless", nameVisibility: "gm"},
		},
	}
	gm := cv.For(ws.Viewer{IsGM: true})
	if len(gm.Combatants) != 3 || gm.TurnCombatantID == nil || *gm.TurnCombatantID != "h" || gm.Combatants[2].Name != "Nameless" {
		t.Errorf("GM view = %+v", gm)
	}
	pl := cv.For(ws.Viewer{PlayerID: "p"})
	if len(pl.Combatants) != 2 || pl.TurnCombatantID != nil || pl.Combatants[1].Name != "" || pl.Combatants[1].ID != "n" {
		t.Errorf("player view = %+v", pl)
	}
	cv.Turn = "v"
	if pl := cv.For(ws.Viewer{PlayerID: "p"}); pl.TurnCombatantID == nil || *pl.TurnCombatantID != "v" {
		t.Errorf("a visible turn must be kept: %+v", pl)
	}
	cv.Turn = ""
	if pl := cv.For(ws.Viewer{PlayerID: "p"}); pl.TurnCombatantID != nil {
		t.Errorf("no turn must stay null: %+v", pl)
	}
	empty := CombatView{SceneID: "s", Round: 1}.For(ws.Viewer{})
	if empty.Combatants == nil {
		t.Error("combatants must not be nil")
	}
}

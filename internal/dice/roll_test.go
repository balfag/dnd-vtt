package dice

import (
	"encoding/json"
	"strings"
	"testing"
)

// seqRoller returns the given values in order and fails the test when they run out
// or a value is outside [1, sides].
type seqRoller struct {
	t     *testing.T
	vals  []int
	calls int
}

func (r *seqRoller) Roll(sides int) int {
	r.t.Helper()
	if r.calls >= len(r.vals) {
		r.t.Fatalf("roller ran out of values after %d calls", r.calls)
	}
	v := r.vals[r.calls]
	r.calls++
	if v < 1 || v > sides {
		r.t.Fatalf("value %d is outside [1, %d]", v, sides)
	}
	return v
}

func roll(t *testing.T, formula string, vals ...int) (Result, *seqRoller) {
	t.Helper()
	f, err := Parse(formula)
	if err != nil {
		t.Fatal(err)
	}
	r := &seqRoller{t: t, vals: vals}
	return f.Roll(r), r
}

func values(dg *DiceGroup) []int {
	out := make([]int, len(dg.Dice))
	for i, d := range dg.Dice {
		out[i] = d.Value
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRollAdvantage(t *testing.T) {
	res, _ := roll(t, "2d20kh1+5", 7, 15)
	dg := res.Terms[0].Dice
	if !equalInts(values(dg), []int{7, 15}) || !dg.Dice[0].Dropped || dg.Dice[1].Dropped {
		t.Fatalf("dice = %+v, want 7 dropped and 15 kept", dg.Dice)
	}
	if res.Total != 20 || res.Formula != "2d20kh1+5" {
		t.Errorf("result = %+v, want total 20", res)
	}
	if res.Terms[0].Value != 15 || res.Terms[1].Value != 5 || res.Terms[1].Dice != nil {
		t.Errorf("terms = %+v", res.Terms)
	}
}

func TestRollDropLowestTieDropsLargerIndex(t *testing.T) {
	res, _ := roll(t, "4d6dl1", 3, 1, 6, 1)
	dice := res.Terms[0].Dice.Dice
	for i, wantDropped := range []bool{false, false, false, true} {
		if dice[i].Dropped != wantDropped {
			t.Errorf("die %d dropped = %v, want %v (the second 1 goes)", i, dice[i].Dropped, wantDropped)
		}
	}
	if res.Total != 10 {
		t.Errorf("total = %d, want 10", res.Total)
	}
}

func TestRollKeepAndDropVariants(t *testing.T) {
	tests := []struct {
		formula string
		vals    []int
		dropped []bool
		total   int
	}{
		{"3d6kh2", []int{4, 2, 5}, []bool{false, true, false}, 9},
		{"3d6kl2", []int{4, 2, 5}, []bool{false, false, true}, 6},
		{"3d6dh1", []int{4, 2, 5}, []bool{false, false, true}, 6},
		{"3d6dl2", []int{4, 2, 5}, []bool{true, true, false}, 5},
		// Ties: the lower index is kept by kh/kl, the higher is dropped by dh/dl.
		{"3d6kh1", []int{5, 5, 1}, []bool{false, true, true}, 5},
		{"3d6kl1", []int{2, 2, 6}, []bool{false, true, true}, 2},
		{"3d6dh1", []int{5, 5, 1}, []bool{false, true, false}, 6},
		{"3d6kh3", []int{1, 2, 3}, []bool{false, false, false}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			res, _ := roll(t, tt.formula, tt.vals...)
			for i, d := range res.Terms[0].Dice.Dice {
				if d.Dropped != tt.dropped[i] {
					t.Errorf("die %d (%d) dropped = %v, want %v", i, d.Value, d.Dropped, tt.dropped[i])
				}
			}
			if res.Total != tt.total {
				t.Errorf("total = %d, want %d", res.Total, tt.total)
			}
		})
	}
}

func TestRollRerollHappensOnce(t *testing.T) {
	res, r := roll(t, "2d6r<=2+3", 1, 5, 2)
	dice := res.Terms[0].Dice.Dice
	if dice[0].RerolledFrom != 1 || dice[0].Value != 2 {
		t.Errorf("die 0 = %+v, want rerolled from 1 to 2 (not rerolled again)", dice[0])
	}
	if dice[1].RerolledFrom != 0 || dice[1].Value != 5 {
		t.Errorf("die 1 = %+v, want 5 untouched", dice[1])
	}
	if res.Total != 10 {
		t.Errorf("total = %d, want 10", res.Total)
	}
	if r.calls != 3 {
		t.Errorf("roller called %d times, want 3", r.calls)
	}
}

func TestRollRerollConditions(t *testing.T) {
	tests := []struct {
		formula string
		vals    []int
		want    []int // values after the reroll pass
		rerolls int
	}{
		{"3d6r2", []int{2, 3, 2, 6, 6}, []int{6, 3, 6}, 2},
		{"3d6r<3", []int{1, 2, 3, 6, 6}, []int{6, 6, 3}, 2},
		{"3d6r>4", []int{5, 6, 4, 1, 2}, []int{1, 2, 4}, 2},
		{"3d6r>=5", []int{5, 6, 4, 1, 2}, []int{1, 2, 4}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			res, r := roll(t, tt.formula, tt.vals...)
			if got := values(res.Terms[0].Dice); !equalInts(got, tt.want) {
				t.Errorf("values = %v, want %v", got, tt.want)
			}
			if r.calls != 3+tt.rerolls {
				t.Errorf("roller called %d times, want %d", r.calls, 3+tt.rerolls)
			}
		})
	}
}

func TestRollExplosions(t *testing.T) {
	res, r := roll(t, "3d6!-1", 6, 2, 6, 6, 3, 1)
	dg := res.Terms[0].Dice
	if got := values(dg); !equalInts(got, []int{6, 2, 6, 6, 3, 1}) {
		t.Fatalf("values = %v, want [6 2 6 6 3 1]", got)
	}
	for i, wantExploded := range []bool{false, false, false, true, true, true} {
		if dg.Dice[i].Exploded != wantExploded {
			t.Errorf("die %d exploded = %v, want %v", i, dg.Dice[i].Exploded, wantExploded)
		}
	}
	if dg.Count != 3 || dg.Sides != 6 {
		t.Errorf("group = %d d %d, want count 3 (before explosions) and 6 sides", dg.Count, dg.Sides)
	}
	if res.Terms[0].Value != 24 || res.Total != 23 {
		t.Errorf("group value %d, total %d; want 24 and 23", res.Terms[0].Value, res.Total)
	}
	if r.calls != 6 {
		t.Errorf("roller called %d times, want 6", r.calls)
	}
}

func TestRollExplosionLimit(t *testing.T) {
	vals := make([]int, 102)
	for i := range vals {
		vals[i] = 2
	}
	res, r := roll(t, "1d2!", vals...)
	if n := len(res.Terms[0].Dice.Dice); n != 101 {
		t.Errorf("dice = %d, want 101 (1 rolled and 100 added)", n)
	}
	if r.calls != 101 {
		t.Errorf("roller called %d times, want 101", r.calls)
	}
	if res.Total != 202 {
		t.Errorf("total = %d, want 202", res.Total)
	}
}

func TestRollExplosionOfRerolledDie(t *testing.T) {
	// The rerolled value counts: 1 is rerolled to 6, which explodes.
	res, _ := roll(t, "1d6r1!", 1, 6, 3)
	dice := res.Terms[0].Dice.Dice
	if len(dice) != 2 || dice[0].RerolledFrom != 1 || !dice[1].Exploded || res.Total != 9 {
		t.Errorf("dice = %+v, total %d; want a rerolled 6 and an exploded 3", dice, res.Total)
	}
}

func TestRollOrderKeepsAfterExplosions(t *testing.T) {
	// reroll -> explode -> keep: the added die takes part in the selection.
	res, _ := roll(t, "2d6!kh1", 6, 2, 5)
	dice := res.Terms[0].Dice.Dice
	if len(dice) != 3 || !dice[1].Dropped || !dice[2].Dropped || dice[0].Dropped || res.Total != 6 {
		t.Errorf("dice = %+v, total %d; want only the first 6 kept", dice, res.Total)
	}
}

func TestRollNegativeAndConstants(t *testing.T) {
	res, _ := roll(t, "-2+d4", 3)
	if res.Total != 1 || res.Terms[0].Sign != -1 || res.Terms[0].Value != 2 || res.Terms[1].Sign != 1 {
		t.Errorf("result = %+v, want total 1 with a negative first term", res)
	}

	res, r := roll(t, "10")
	if res.Total != 10 || res.Terms[0].Dice != nil || r.calls != 0 {
		t.Errorf("result = %+v, want a plain 10", res)
	}

	res, _ = roll(t, "1d20-1d4", 10, 3)
	if res.Total != 7 {
		t.Errorf("1d20-1d4 total = %d, want 7", res.Total)
	}
}

func TestResultJSON(t *testing.T) {
	res, _ := roll(t, "2d6r<=2+3", 1, 5, 2)
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"rerolledFrom":1`) {
		t.Errorf("JSON %s lacks rerolledFrom", s)
	}
	if strings.Contains(s, "dropped") || strings.Contains(s, "exploded") {
		t.Errorf("JSON %s has flags of dice that have none", s)
	}
	var back Result
	if err := json.Unmarshal(b, &back); err != nil || back.Formula != "2d6r<=2+3" || back.Total != 10 {
		t.Errorf("JSON %s does not decode back to the same result: %+v, %v", s, back, err)
	}

	res, _ = roll(t, "2d20kh1", 7, 15)
	b, _ = json.Marshal(res)
	if !strings.Contains(string(b), `"dropped":true`) {
		t.Errorf("JSON %s lacks dropped", b)
	}
}

func TestCryptoRoller(t *testing.T) {
	r := CryptoRoller()
	var seen [7]int
	for range 6000 {
		v := r.Roll(6)
		if v < 1 || v > 6 {
			t.Fatalf("Roll(6) = %d", v)
		}
		seen[v]++
	}
	for face := 1; face <= 6; face++ {
		if seen[face] < 700 {
			t.Errorf("face %d came up %d times in 6000 rolls, want at least 700", face, seen[face])
		}
	}
}

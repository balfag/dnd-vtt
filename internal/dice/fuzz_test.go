package dice

import (
	"errors"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, tt := range validFormulas {
		f.Add(tt.in)
	}
	for _, s := range []string{
		"", "2d6+", "2d", "2d1", "0d6", "101d6", "2d6kh3", "2d6kh1kl1", "1d6r<=6", "1 0d6", "2d6x", "2d6ж",
		"100d6+100d6+1d6", "1d6r>=1", "d", "-", "+-", "1d%!!", "99999999999999999999d6",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		formula, err := Parse(s)
		if err != nil {
			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) error %v is not a *ParseError", s, err)
			}
			if n := len([]rune(s)); perr.Pos < 0 || perr.Pos > n {
				t.Fatalf("Parse(%q) position %d is outside [0, %d]", s, perr.Pos, n)
			}
			return
		}

		canonical := formula.String()
		again, err := Parse(canonical)
		if err != nil {
			t.Fatalf("canonical form %q of %q does not parse: %v", canonical, s, err)
		}
		if again.String() != canonical {
			t.Fatalf("canonical form is not stable: %q then %q", canonical, again.String())
		}

		res := formula.Roll(CryptoRoller())
		lo, hi := bounds(formula)
		if res.Total < lo || res.Total > hi {
			t.Fatalf("%q rolled %d, outside [%d, %d]", canonical, res.Total, lo, hi)
		}
	})
}

// bounds returns a coarse range of totals: a dice group is worth 0 to
// (count + added explosions) * sides, a constant is exact.
func bounds(f Formula) (lo, hi int) {
	for _, t := range f.terms {
		min, max := t.constant, t.constant
		if t.group != nil {
			min, max = 0, (t.group.count+maxExplosions)*t.group.sides
		}
		if t.sign > 0 {
			lo, hi = lo+min, hi+max
		} else {
			lo, hi = lo-max, hi-min
		}
	}
	return lo, hi
}

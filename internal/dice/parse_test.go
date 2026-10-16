package dice

import (
	"errors"
	"strings"
	"testing"
)

var validFormulas = []struct{ in, want string }{
	{"d20", "1d20"},
	{"2d20kh1 + 5", "2d20kh1+5"},
	{"4d6dl1", "4d6dl1"},
	{"8d6", "8d6"},
	{"1d%", "1d100"},
	{"2d6r<=2+3", "2d6r<=2+3"},
	{"3d6!-1", "3d6!-1"},
	{"2D20KH", "2d20kh1"},
	{"d6!r1kh1", "1d6kh1r1!"},
	{"10", "10"},
	{"-2+d4", "-2+1d4"},
	{"+2d6", "2d6"},
	{"d%kh", "1d100kh1"},
	{"  2 d 6 kh 1 + 3 ", "2d6kh1+3"},
	{"2d6r>=5", "2d6r>=5"},
	{"2d6r<3", "2d6r<3"},
	{"2d6r>4", "2d6r>4"},
	{"4d6dh2", "4d6dh2"},
	{"3d8kl2", "3d8kl2"},
	{"1d20-1d4", "1d20-1d4"},
	{"1000000", "1000000"},
	{"0", "0"},
	{"100d1000", "100d1000"},
}

func TestParseValid(t *testing.T) {
	for _, tt := range validFormulas {
		t.Run(tt.in, func(t *testing.T) {
			f, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got := f.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
			again, err := Parse(f.String())
			if err != nil || again.String() != tt.want {
				t.Fatalf("canonical form %q does not parse back: %v", tt.want, err)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	terms21 := strings.Repeat("1+", 20) + "1"
	tests := []struct {
		name string
		in   string
		pos  int
	}{
		{"empty", "", 0},
		{"only spaces", "   ", 3},
		{"trailing sign", "2d6+", 4},
		{"double sign", "2d6+-1", 4},
		{"lone sign", "-", 1},
		{"no sides", "2d", 2},
		{"d alone", "d", 1},
		{"one side", "2d1", 2},
		{"too many sides", "1d1001", 2},
		{"zero count", "0d6", 0},
		{"count over 100", "101d6", 0},
		{"keep over count", "2d6kh3", 3},
		{"keep zero", "2d6kh0", 3},
		{"second keep", "2d6kh1kl1", 6},
		{"keep without h or l", "2d6k", 3},
		{"unknown d modifier", "2d6d6", 3},
		{"reroll covers every side", "1d6r<=6", 3},
		{"reroll covers every side, ge", "1d6r>=1", 3},
		{"reroll number over sides", "1d2r<3", 3},
		{"reroll zero", "1d6r0", 3},
		{"reroll without number", "2d6r", 4},
		{"second reroll", "2d6r1r2", 5},
		{"second explode", "2d6!!", 4},
		{"space inside a number", "1 0d6", 2},
		{"unknown character", "2d6x", 3},
		{"non-ASCII character is one rune", "2d6ж", 3},
		{"non-ASCII after a rune", "ж", 0},
		{"constant too large", "1000001", 0},
		{"huge constant", "99999999999999999999999", 0},
		{"huge count", "99999999999999999999d6", 0},
		{"too long", strings.Repeat("1", 201), 200},
		// 101 runes are 202 bytes: the limit counts runes, so this is short enough.
		{"length is counted in runes", strings.Repeat("ж", 101), 0},
		{"21 terms", terms21, 40},
		{"sum of dice over 200", "100d6+100d6+1d6", 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.in)
			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) error = %v, want *ParseError", tt.in, err)
			}
			if perr.Pos != tt.pos {
				t.Errorf("Parse(%q) pos = %d (%v), want %d", tt.in, perr.Pos, perr, tt.pos)
			}
		})
	}
}

func TestParseLimitsAreInclusive(t *testing.T) {
	for _, in := range []string{
		strings.Repeat(" ", 199) + "1",   // 200 characters
		strings.Repeat("1+", 19) + "1",   // 20 terms
		"100d6+100d6",                    // 200 dice
		"1000000",                        // the largest constant
		"1d1000kh1r1!",                   // everything at once
		strings.Repeat(" ", 195) + "1d6", // spaces count towards the length
		"1d2r1",                          // reroll covering one of two sides
		"100d6dl99",                      // keep at its upper bound
	} {
		if _, err := Parse(in); err != nil {
			t.Errorf("Parse(%.30q) = %v, want nil", in, err)
		}
	}
}

func TestParseErrorMessage(t *testing.T) {
	_, err := Parse("2d6+")
	if err == nil || !strings.HasPrefix(err.Error(), "dice: ") || !strings.HasSuffix(err.Error(), " at 4") {
		t.Fatalf("error = %v, want the form \"dice: <msg> at 4\"", err)
	}
}

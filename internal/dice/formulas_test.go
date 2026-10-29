package dice

import (
	"encoding/json"
	"os"
	"testing"
)

// TestFormulasFile runs the cases shared with the web client
// (web/src/shared/dice/parse.test.ts): both parsers must give the same answers.
func TestFormulasFile(t *testing.T) {
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	raw, err := root.ReadFile("formulas.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Formula string `json:"formula"`
		OK      bool   `json:"ok"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 60 {
		t.Fatalf("formulas.json has %d cases, want at least 60", len(cases))
	}
	for _, c := range cases {
		_, err := Parse(c.Formula)
		if (err == nil) != c.OK {
			t.Errorf("Parse(%q): err = %v, want ok = %v", c.Formula, err, c.OK)
		}
	}
}

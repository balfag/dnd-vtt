package engine

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

func osDir() fs.FS { return os.DirFS("../data") }

// copyData returns an in-memory copy of data/ that a test can modify.
func copyData(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	entries, err := fs.ReadDir(osDir(), ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := fs.ReadFile(osDir(), e.Name())
		if err != nil {
			t.Fatal(err)
		}
		m[e.Name()] = &fstest.MapFile{Data: b}
	}
	return m
}

func loadComp(t *testing.T) *Compendium {
	t.Helper()
	c, err := Load(os.DirFS("../data"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func seq(vals ...int) *SeqRoller { return &SeqRoller{Values: vals} }

// done fails the test if the sequence was misused or not fully consumed:
// every die the engine rolls must be accounted for in the expected numbers.
func done(t *testing.T, r *SeqRoller) {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("dice sequence: %v", r.Err)
	}
	if r.Remaining() != 0 {
		t.Fatalf("%d unused dice in sequence", r.Remaining())
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func sameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	g := map[string]int{}
	for _, v := range got {
		g[v]++
	}
	for _, v := range want {
		g[v]--
	}
	for k, n := range g {
		if n != 0 {
			t.Errorf("%s: got %v, want %v (diff at %q)", what, got, want, k)
			return
		}
	}
}

var fighterAbilities = map[string]int{"str": 16, "dex": 14, "con": 15, "int": 8, "wis": 12, "cha": 10}
var wizardAbilities = map[string]int{"str": 8, "dex": 14, "con": 14, "int": 16, "wis": 12, "cha": 10}

// fighter3 is Human / Soldier / Champion with the choices a player would make.
func fighter3(species string) Character {
	ch := Character{
		Name: "Fighter", Species: species, Background: "soldier", Class: "fighter", Level: 3,
		Abilities: fighterAbilities,
		Choices: map[string][]string{
			"fighter-skills":     {"acrobatics", "perception"},
			"fighting-style":     {"defense"},
			"weapon-mastery":     {"longsword", "shortbow", "javelin"},
			"fighter-subclass":   {"champion"},
			"soldier-gaming-set": {"dice-set"},
		},
		Equipped: []string{"chain-mail", "shield", "longsword"},
	}
	if species == "human" {
		ch.Choices["human-skillful"] = []string{"insight"}
		ch.Choices["human-versatile"] = []string{"alert"}
	}
	return ch
}

// wizard is Elf (Wood Elf) / Acolyte / Evoker at the given level (3 or 5).
func wizard(level int) Character {
	ch := Character{
		Name: "Wizard", Species: "elf", Background: "acolyte", Class: "wizard", Level: level,
		Abilities: wizardAbilities,
		Choices: map[string][]string{
			"elf-keen-senses": {"perception"},
			"elven-lineage":   {"wood-elf"},
			"wizard-skills":   {"arcana", "investigation"},
			"scholar":         {"arcana"},
			"mi-ability":      {"int"},
			"mi-cantrips":     {"sacred-flame", "light"},
			"mi-spell":        {"cure-wounds"},
			"wizard-subclass": {"evoker"},
		},
	}
	if level >= 4 {
		// Wis +2 so the casting numbers (Int 16) stay the same as at level 3.
		ch.Choices["asi-4"] = []string{"asi:wis+2"}
	}
	return ch
}

func mustSheet(t *testing.T, c *Compendium, ch Character) *Sheet {
	t.Helper()
	s, err := c.BuildSheet(ch)
	if err != nil {
		t.Fatalf("BuildSheet(%s): %v", ch.Name, err)
	}
	return s
}

func mustCaster(t *testing.T, s *Sheet) *Caster {
	t.Helper()
	cs, err := CasterFromSheet("Wizard", s)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func monster(t *testing.T, c *Compendium, id string) *Target {
	t.Helper()
	m, ok := c.Monsters[id]
	if !ok {
		t.Fatalf("no monster %s", id)
	}
	return TargetFromMonster(m, m.Name)
}

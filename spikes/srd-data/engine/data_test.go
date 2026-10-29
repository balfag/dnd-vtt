package engine

import (
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDiceExpressions(t *testing.T) {
	vars := map[string]int{"spellmod": 3, "prof": 2}
	p, err := ParseExpr("2d8 + @spellmod", vars)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "string", p.String(), "2d8+3")
	// +1d6 per slot above: 8d6 scaled twice -> 10d6; merged groups keep one die size.
	base, _ := ParseExpr("8d6", nil)
	per, _ := ParseExpr("1d6", nil)
	base.AddN(per, 2)
	eq(t, "scaled", base.String(), "10d6")
	neg, _ := ParseExpr("1d4 - 1", nil)
	eq(t, "minus", neg.String(), "1d4-1")
	if _, err := ParseExpr("@nope", nil); err == nil {
		t.Error("unknown variable must fail")
	}
	// A sequence that runs dry or holds an impossible value is reported, not hidden.
	r := seq(7)
	r.Roll(6)
	if r.Err == nil {
		t.Error("d6 result 7 must be flagged")
	}
	r = seq()
	r.Roll(6)
	if r.Err == nil {
		t.Error("exhausted sequence must be flagged")
	}
}

func TestCompendiumCounts(t *testing.T) {
	c := loadComp(t)
	eq(t, "spells", len(c.Spells), 20)
	eq(t, "classes", len(c.Classes), 2)
	eq(t, "subclasses", len(c.Subclasses), 2)
	eq(t, "species", len(c.Species), 3)
	eq(t, "backgrounds", len(c.Backgrounds), 2)
	eq(t, "items", len(c.Items), 5)
	eq(t, "monsters", len(c.Monsters), 3)
	for id, s := range c.Spells {
		if s.Source != "srd-5.2.1" {
			t.Errorf("%s: source %q", id, s.Source)
		}
	}
}

// Content is validated strictly so that homebrew typos fail on load.
func TestLoadRejectsBadRecords(t *testing.T) {
	good, err := Load(osDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = good
	cases := map[string]string{
		"unknown field": `[{"schemaVersion":1,"id":"x","name":"X","source":"homebrew","kind":"gear","cost":1}]`,
		"old version":   `[{"schemaVersion":0,"id":"x","name":"X","source":"homebrew","kind":"gear"}]`,
		"bad source":    `[{"schemaVersion":1,"id":"x","name":"X","source":"phb","kind":"gear"}]`,
		"duplicate id":  `[{"schemaVersion":1,"id":"x","name":"X","source":"homebrew","kind":"gear"},{"schemaVersion":1,"id":"x","name":"Y","source":"homebrew","kind":"gear"}]`,
	}
	for name, items := range cases {
		fsys := copyData(t)
		fsys["items.json"] = &fstest.MapFile{Data: []byte(items)}
		if _, err := Load(fsys); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// A valid homebrew item loads next to SRD content.
	fsys := copyData(t)
	fsys["items.json"] = &fstest.MapFile{Data: []byte(`[{"schemaVersion":1,"id":"moon-blade","name":"Moon Blade","source":"homebrew","kind":"weapon","costGp":100,"weightLb":3,"weapon":{"category":"martial","ranged":false,"damage":{"dice":"1d8","type":"radiant"},"properties":["finesse"]}}]`)}
	c, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "homebrew source", c.Items["moon-blade"].Source, "homebrew")
}

// Coverage: which effects are plain data and which need a code handler.
// The expected handler list below is what REPORT.md quotes.
func TestCoverageAndHandlers(t *testing.T) {
	c := loadComp(t)
	cov := c.Coverage()

	var spellSpecial, spellClean int
	spellHandlers := map[string]int{}
	var declSpell, specSpell int
	for key, e := range cov {
		if !strings.HasPrefix(key, "spell/") {
			continue
		}
		declSpell += e.Declarative
		specSpell += e.Special
		if e.Special == 0 {
			spellClean++
		} else {
			spellSpecial++
		}
		for _, h := range e.Handlers {
			spellHandlers[h]++
		}
	}
	// 14 of 20 spells are fully declarative; 6 contain a handler (see REPORT.md).
	eq(t, "spells fully declarative", spellClean, 14)
	eq(t, "spells with a handler", spellSpecial, 6)
	eq(t, "ignite_objects", spellHandlers["ignite_objects"], 3)
	eq(t, "push_objects", spellHandlers["push_objects"], 1)
	eq(t, "damage_objects", spellHandlers["damage_objects"], 1)
	eq(t, "counterspell", spellHandlers["counterspell"], 1)
	t.Logf("spell effect nodes: %d declarative, %d special", declSpell, specSpell)

	other := map[string]bool{}
	for key, e := range cov {
		if strings.HasPrefix(key, "spell/") {
			continue
		}
		for _, h := range e.Handlers {
			other[key+"="+h] = true
		}
	}
	var got []string
	for k := range other {
		got = append(got, k)
	}
	sort.Strings(got)
	sameSet(t, "non-spell handlers", got, []string{
		"class/fighter/tactical-mind=tactical_mind",
		"class/fighter/tactical-shift=tactical_shift",
		"class/wizard/arcane-recovery=arcane_recovery",
		"class/wizard/memorize-spell=memorize_spell",
		"subclass/champion/remarkable-athlete=crit_move",
		"subclass/evoker/evocation-savant=spellbook_grant",
		"species/human/resourceful=heroic_inspiration_on_rest",
		"feat/alert=initiative_swap",
	})
	// Monster actions need no handlers.
	for key, e := range cov {
		if strings.HasPrefix(key, "monster/") && e.Special != 0 {
			t.Errorf("%s uses a handler", key)
		}
	}
}

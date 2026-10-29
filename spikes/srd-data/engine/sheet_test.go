package engine

import (
	"fmt"
	"strings"
	"testing"
)

// Fighter 3: Human, Soldier, Champion. Str 16 Dex 14 Con 15 Int 8 Wis 12 Cha 10.
func TestFighter3Sheet(t *testing.T) {
	c := loadComp(t)
	s := mustSheet(t, c, fighter3("human"))

	// Proficiency bonus: 2 + (level-1)/4 = 2 for levels 1-4.
	eq(t, "prof", s.ProfBonus, 2)
	// Modifiers: floor((score-10)/2): 16->+3, 14->+2, 15->+2, 8->-1, 12->+1, 10->0.
	for ab, want := range map[string]int{"str": 3, "dex": 2, "con": 2, "int": -1, "wis": 1, "cha": 0} {
		eq(t, "mod "+ab, s.Mods[ab], want)
	}
	// Saves: Fighter is proficient in Str and Con: +3+2=5, +2+2=4; others are the bare modifier.
	for ab, want := range map[string]int{"str": 5, "con": 4, "dex": 2, "int": -1, "wis": 1, "cha": 0} {
		eq(t, "save "+ab, s.Saves[ab], want)
	}
	// Skill proficiencies: Soldier gives Athletics and Intimidation; the Fighter picks
	// Acrobatics and Perception; Human Skillful adds Insight.
	// Athletics Str+3+2=5, Intimidation Cha+0+2=2, Acrobatics Dex+2+2=4,
	// Perception Wis+1+2=3, Insight Wis+1+2=3. Unproficient: Stealth Dex=2, Arcana Int=-1.
	for sk, want := range map[string]int{"athletics": 5, "intimidation": 2, "acrobatics": 4, "perception": 3, "insight": 3, "stealth": 2, "arcana": -1} {
		eq(t, "skill "+sk, s.Skills[sk], want)
	}
	eq(t, "athletics proficient", s.SkillProf["athletics"], "proficient")
	eq(t, "stealth proficient", s.SkillProf["stealth"], "")
	// Passive Perception = 10 + 3.
	eq(t, "passive perception", s.Passive["perception"], 13)
	// AC: Chain Mail 16 (heavy: no Dex), Shield +2, Fighting Style Defense +1 (armor worn) = 19.
	eq(t, "AC", s.AC, 19)
	// HP: level 1 = 10 (die) + 2 (Con) = 12; levels 2 and 3: fixed 10/2+1 = 6, +2 = 8 each.
	// 12 + 8 + 8 = 28.
	eq(t, "HP", s.HPMax, 28)
	// Initiative: Dex +2, plus proficiency bonus from the Alert feat (Human Versatile) = +4;
	// Champion Remarkable Athlete gives Advantage.
	eq(t, "initiative", s.Initiative, 4)
	eq(t, "initiative advantage", s.Flags["initiative"], "advantage")
	eq(t, "athletics advantage", s.Flags["skill.athletics"], "advantage")
	// Champion Improved Critical: crit range 19.
	eq(t, "crit range", s.Values["crit.range"], 19)
	eq(t, "speed", s.Speed, 30)
	// Second Wind: class table column secondWindUses = 2 at level 3.
	eq(t, "second wind uses", s.Uses["second-wind"], 2)
	// Longsword, Str-based, proficient (martial): +3 +2 = +5; damage 1d8 + 3; two-handed 1d10 + 3.
	if len(s.Attacks) != 1 {
		t.Fatalf("attacks = %+v", s.Attacks)
	}
	a := s.Attacks[0]
	eq(t, "attack bonus", a.Bonus, 5)
	eq(t, "damage", a.Damage, "1d8 + 3")
	eq(t, "two-handed damage", a.DamageTwo, "1d10 + 3")
	eq(t, "damage type", a.DamageTyp, "slashing")
	if s.Spell != nil {
		t.Error("fighter must have no spellcasting")
	}
	sameSet(t, "feats", s.Feats, []string{"savage-attacker", "alert", "defense"})
	// Specials that the sheet relies on but does not interpret.
	sameSet(t, "handlers", s.Handlers, []string{"tactical_mind", "crit_move", "heroic_inspiration_on_rest", "initiative_swap"})
	if len(s.Conflicts) != 0 {
		t.Errorf("conflicts: %v", s.Conflicts)
	}
}

// A Dwarf Fighter: Dwarven Toughness is a per-level HP modifier.
func TestDwarfFighterHP(t *testing.T) {
	c := loadComp(t)
	s := mustSheet(t, c, fighter3("dwarf"))
	// Base 28 as above, +1 for each of 3 levels = 31.
	eq(t, "HP", s.HPMax, 31)
	// Initiative has no Alert bonus here: Dex +2.
	eq(t, "initiative", s.Initiative, 2)
	eq(t, "darkvision", s.Senses["darkvision"], 120)
	sameSet(t, "resist", s.Resist, []string{"poison"})
}

// Wizard 3: Elf (Wood Elf), Acolyte, Evoker. Str 8 Dex 14 Con 14 Int 16 Wis 12 Cha 10.
func TestWizard3Sheet(t *testing.T) {
	c := loadComp(t)
	s := mustSheet(t, c, wizard(3))

	eq(t, "prof", s.ProfBonus, 2)
	// Mods: 8->-1, 14->+2, 14->+2, 16->+3, 12->+1, 10->0.
	for ab, want := range map[string]int{"str": -1, "dex": 2, "con": 2, "int": 3, "wis": 1, "cha": 0} {
		eq(t, "mod "+ab, s.Mods[ab], want)
	}
	// Saves: Wizard is proficient in Int and Wis: 3+2=5, 1+2=3.
	for ab, want := range map[string]int{"int": 5, "wis": 3, "dex": 2, "con": 2, "str": -1, "cha": 0} {
		eq(t, "save "+ab, s.Saves[ab], want)
	}
	// Skills: Acolyte Insight + Religion, Wizard picks Arcana + Investigation, Elf Keen Senses
	// picks Perception. Arcana has Expertise (Scholar): 3 + 2*2 = 7. Investigation 3+2=5,
	// Religion 3+2=5, Insight 1+2=3, Perception 1+2=3. History (no proficiency) = 3.
	for sk, want := range map[string]int{"arcana": 7, "investigation": 5, "religion": 5, "insight": 3, "perception": 3, "history": 3} {
		eq(t, "skill "+sk, s.Skills[sk], want)
	}
	eq(t, "arcana", s.SkillProf["arcana"], "expertise")
	// No armor: AC = 10 + Dex 2 = 12.
	eq(t, "AC", s.AC, 12)
	// HP: level 1 = 6 + 2 = 8; levels 2, 3: 6/2+1 = 4, +2 = 6 each. 8 + 6 + 6 = 20.
	eq(t, "HP", s.HPMax, 20)
	// Wood Elf lineage: Speed 35. Elf Darkvision 60.
	eq(t, "speed", s.Speed, 35)
	eq(t, "darkvision", s.Senses["darkvision"], 60)
	// Spellcasting (Int): DC = 8 + 2 + 3 = 13, attack = 2 + 3 = +5.
	// Level 3 row: 3 cantrips, 6 prepared, slots 4 x L1, 2 x L2.
	if s.Spell == nil {
		t.Fatal("no spellcasting")
	}
	eq(t, "spell DC", s.Spell.DC, 13)
	eq(t, "spell attack", s.Spell.Attack, 5)
	eq(t, "cantrips", s.Spell.Cantrips, 3)
	eq(t, "prepared", s.Spell.Prepared, 6)
	eq(t, "slots", fmt.Sprint(s.Spell.Slots), "[4 2]")
	// Spells granted by choices: Wood Elf (Druidcraft; Longstrider at level 3),
	// Magic Initiate (Cleric): Sacred Flame, Light, and Cure Wounds (once per Long Rest).
	sameSet(t, "granted spells", s.GrantedSpells, []string{"druidcraft", "longstrider", "sacred-flame", "light", "cure-wounds"})
	eq(t, "free cast", s.Uses["free_cast_per_long_rest:cure-wounds"], 1)
	sameSet(t, "handlers", s.Handlers, []string{"arcane_recovery", "spellbook_grant"})
	eq(t, "potent cantrip", s.Flags["cantrip.onMissOrSave"], "set")
}

func TestWizardAC_MageArmor(t *testing.T) {
	c := loadComp(t)
	ch := wizard(3)
	ch.Buffs = []string{"mage-armor"}
	// Mage Armor: base AC 13 + Dex 2 = 15 (replaces 10 + Dex = 12).
	eq(t, "AC with Mage Armor", mustSheet(t, c, ch).AC, 15)
	// The effect only applies without armor: with Chain Mail (16, no Dex) the best base is 16.
	ch.Equipped = []string{"chain-mail"}
	eq(t, "AC in chain mail with Mage Armor", mustSheet(t, c, ch).AC, 16)
}

func TestWizard5Sheet(t *testing.T) {
	c := loadComp(t)
	s := mustSheet(t, c, wizard(5))
	// Proficiency bonus at level 5: 2 + 4/4 = 3.
	eq(t, "prof", s.ProfBonus, 3)
	// Wis 12 + 2 (ASI) = 14 -> +2; Int stays 16.
	eq(t, "wis", s.Abilities["wis"], 14)
	// DC 8 + 3 + 3 = 14; attack 3 + 3 = +6.
	eq(t, "spell DC", s.Spell.DC, 14)
	eq(t, "spell attack", s.Spell.Attack, 6)
	eq(t, "cantrips", s.Spell.Cantrips, 4)
	eq(t, "prepared", s.Spell.Prepared, 9)
	eq(t, "slots", fmt.Sprint(s.Spell.Slots), "[4 3 2]")
	// Level 5 Wood Elf lineage adds Pass without Trace.
	if !contains(s.GrantedSpells, "pass-without-trace") {
		t.Errorf("granted spells = %v", s.GrantedSpells)
	}
}

// The sheet refuses invalid or missing choices instead of guessing.
func TestSheetValidatesChoices(t *testing.T) {
	c := loadComp(t)

	ch := fighter3("human")
	delete(ch.Choices, "fighter-skills")
	if _, err := c.BuildSheet(ch); err == nil || !strings.Contains(err.Error(), "fighter-skills") {
		t.Errorf("missing skills: err = %v", err)
	}

	ch = fighter3("human")
	ch.Choices["fighter-skills"] = []string{"acrobatics", "arcana"} // Arcana is not a Fighter skill
	if _, err := c.BuildSheet(ch); err == nil || !strings.Contains(err.Error(), "arcana") {
		t.Errorf("bad skill: err = %v", err)
	}

	ch = fighter3("human")
	ch.Choices["fighting-style"] = []string{"alert"} // an origin feat, not a Fighting Style
	if _, err := c.BuildSheet(ch); err == nil || !strings.Contains(err.Error(), "fighting_style") {
		t.Errorf("bad fighting style: err = %v", err)
	}

	ch = fighter3("human")
	ch.Choices["weapon-mastery"] = []string{"longsword"} // column weaponMastery = 3 at level 3
	if _, err := c.BuildSheet(ch); err == nil || !strings.Contains(err.Error(), "need 3 picks") {
		t.Errorf("mastery count: err = %v", err)
	}

	ch = fighter3("human")
	ch.Level = 4 // level 4 needs an Ability Score Improvement pick (and a 4th weapon mastery)
	ch.Choices["weapon-mastery"] = append(ch.Choices["weapon-mastery"], "greatsword")
	if _, err := c.BuildSheet(ch); err == nil || !strings.Contains(err.Error(), "asi-4") {
		t.Errorf("missing asi: err = %v", err)
	}
}

// Picking a skill that a background already grants is reported, not silently accepted.
func TestDuplicateProficiencyReported(t *testing.T) {
	c := loadComp(t)
	ch := wizard(3)
	ch.Choices["elf-keen-senses"] = []string{"insight"} // Acolyte already gives Insight
	s := mustSheet(t, c, ch)
	sameSet(t, "conflicts", s.Conflicts, []string{"duplicate proficiency insight"})
}

func TestArmorStrengthRequirementReported(t *testing.T) {
	c := loadComp(t)
	ch := fighter3("human")
	ch.Abilities = map[string]int{"str": 12, "dex": 14, "con": 15, "int": 8, "wis": 12, "cha": 10}
	s := mustSheet(t, c, ch)
	if len(s.Conflicts) != 1 || !strings.Contains(s.Conflicts[0], "Chain Mail needs Str 13") {
		t.Errorf("conflicts = %v", s.Conflicts)
	}
}

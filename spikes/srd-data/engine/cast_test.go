package engine

import (
	"fmt"
	"strings"
	"testing"
)

// Dice order inside one cast: attack d20 first, then damage dice; for saving-throw
// spells the damage is rolled once, before the saves, then each target's d20 in target order.

func cast(t *testing.T, c *Compendium, cs *Caster, id string, slot int, targets []*Target, r *SeqRoller, mod func(*CastRequest)) *CastResult {
	t.Helper()
	req := CastRequest{SpellID: id, Slot: slot, Targets: targets, Roller: r}
	if mod != nil {
		mod(&req)
	}
	res, err := c.Cast(cs, req)
	if err != nil {
		t.Fatalf("Cast %s: %v", id, err)
	}
	return res
}

func TestFireBoltHitAtLevel3(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	goblin := monster(t, c, "goblin-warrior")
	r := seq(12, 7)
	res := cast(t, c, cs, "fire-bolt", 0, []*Target{goblin}, r, nil)
	done(t, r)
	o := res.Outcomes[0]
	// Attack: d20 12 + spell attack 5 = 17 >= Goblin Warrior AC 15 -> hit.
	eq(t, "attack total", o.Attack.Total, 17)
	eq(t, "hit", o.AttackHit, true)
	// Level 3 is below the first cantrip tier (5): damage 1d10 -> rolled 7.
	eq(t, "formula", o.Damage[0].Formula, "1d10")
	eq(t, "damage", o.DamageRaw, 7)
	res.ApplyRecommended([]*Target{goblin})
	// Goblin Warrior has 10 HP: 10 - 7 = 3.
	eq(t, "goblin HP", goblin.HP, 3)
	sameSet(t, "unhandled", res.Unhandled, []string{"ignite_objects"})
}

func TestFireBoltCritDoublesDice(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	ogre := monster(t, c, "ogre")
	r := seq(20, 4, 9)
	res := cast(t, c, cs, "fire-bolt", 0, []*Target{ogre}, r, nil)
	done(t, r)
	o := res.Outcomes[0]
	// Natural 20 hits and is a Critical Hit: the damage dice are doubled -> 2d10 = 4 + 9 = 13.
	eq(t, "crit", o.Crit, true)
	eq(t, "damage", o.DamageRaw, 13)
	res.ApplyRecommended([]*Target{ogre})
	eq(t, "ogre HP", ogre.HP, 68-13) // 55
}

func TestFireBoltNatural1MissesEvenAgainstLowAC(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	cs.Flags = nil // no Potent Cantrip: a miss deals nothing and rolls no damage
	ogre := monster(t, c, "ogre")
	r := seq(1)
	res := cast(t, c, cs, "fire-bolt", 0, []*Target{ogre}, r, nil)
	done(t, r) // 1 + 5 = 6 would miss AC 11 anyway; no damage dice are rolled
	eq(t, "hit", res.Outcomes[0].AttackHit, false)
}

func TestFireBoltTier2AtLevel5(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(5)))
	ogre := monster(t, c, "ogre")
	r := seq(12, 4, 9)
	res := cast(t, c, cs, "fire-bolt", 0, []*Target{ogre}, r, nil)
	done(t, r)
	o := res.Outcomes[0]
	// Level 5 reaches the first tier: 2d10. Attack 12 + 6 = 18 >= 11 -> hit. Damage 4 + 9 = 13.
	eq(t, "attack total", o.Attack.Total, 18)
	eq(t, "formula", o.Damage[0].Formula, "2d10")
	eq(t, "damage", o.DamageRaw, 13)
	res.ApplyRecommended([]*Target{ogre})
	eq(t, "ogre HP", ogre.HP, 55)
}

// Evoker's Potent Cantrip: a missed cantrip attack still deals half damage; others deal nothing.
func TestPotentCantripOnMiss(t *testing.T) {
	c := loadComp(t)
	evoker := mustCaster(t, mustSheet(t, c, wizard(3)))
	goblin := monster(t, c, "goblin-warrior")
	r := seq(9, 7)
	res := cast(t, c, evoker, "fire-bolt", 0, []*Target{goblin}, r, nil)
	done(t, r)
	// 9 + 5 = 14 < AC 15 -> miss. Damage is rolled (7) and halved, rounded down: 3.
	eq(t, "hit", res.Outcomes[0].AttackHit, false)
	eq(t, "damage", res.Outcomes[0].DamageRaw, 3)
	res.ApplyRecommended([]*Target{goblin})
	eq(t, "goblin HP", goblin.HP, 7)

	other := mustCaster(t, mustSheet(t, c, wizard(3)))
	other.Flags = nil // a caster without the subclass
	goblin2 := monster(t, c, "goblin-warrior")
	r = seq(9)
	res = cast(t, c, other, "fire-bolt", 0, []*Target{goblin2}, r, nil)
	done(t, r)
	eq(t, "damage without subclass", res.Outcomes[0].DamageRaw, 0)
}

func TestMagicMissile(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	goblin := monster(t, c, "goblin-warrior")
	skel := monster(t, c, "skeleton")

	// Slot 1: three darts, each 1d4 + 1. Rolled d4: 2, 4, 1 -> 3, 5, 2.
	// Darts 1 and 2 hit the goblin (3 + 5 = 8), dart 3 the skeleton (2).
	r := seq(2, 4, 1)
	res := cast(t, c, cs, "magic-missile", 1, []*Target{goblin, skel}, r, func(q *CastRequest) { q.DartTargets = []int{0, 0, 1} })
	done(t, r)
	eq(t, "goblin raw", res.Outcomes[0].DamageRaw, 8)
	eq(t, "skeleton raw", res.Outcomes[1].DamageRaw, 2)
	res.ApplyRecommended([]*Target{goblin, skel})
	eq(t, "goblin HP", goblin.HP, 10-8)
	eq(t, "skeleton HP", skel.HP, 13-2)
	eq(t, "slots left", fmt.Sprint(cs.Slots), "[3 2]")

	// Slot 2: 3 + 1 = 4 darts, all at the Ogre; d4 = 1 each -> 2 each = 8.
	ogre := monster(t, c, "ogre")
	r = seq(1, 1, 1, 1)
	res = cast(t, c, cs, "magic-missile", 2, []*Target{ogre}, r, func(q *CastRequest) { q.DartTargets = []int{0, 0, 0, 0} })
	done(t, r)
	eq(t, "ogre raw", res.Outcomes[0].DamageRaw, 8)
	res.ApplyRecommended([]*Target{ogre})
	eq(t, "ogre HP", ogre.HP, 60)
	eq(t, "slots left", fmt.Sprint(cs.Slots), "[3 1]")
}

func TestFireball(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(5)))
	goblin := monster(t, c, "goblin-warrior")
	ogre := monster(t, c, "ogre")

	// 8d6 is rolled once: 6+5+4+3+2+1+6+4 = 31. Save DC 14.
	// Goblin: Dex 15 -> +2; d20 11 -> 13 < 14 fails -> takes 31.
	// Ogre: Dex 8 -> -1; d20 18 -> 17 >= 14 succeeds -> half of 31 = 15 (round down).
	r := seq(6, 5, 4, 3, 2, 1, 6, 4, 11, 18)
	res := cast(t, c, cs, "fireball", 3, []*Target{goblin, ogre}, r, nil)
	done(t, r)
	eq(t, "DC", res.Outcomes[0].SaveDC, 14)
	eq(t, "goblin save total", res.Outcomes[0].Save.Total, 13)
	eq(t, "goblin passed", res.Outcomes[0].SavePassed, false)
	eq(t, "ogre save total", res.Outcomes[1].Save.Total, 17)
	eq(t, "ogre passed", res.Outcomes[1].SavePassed, true)
	eq(t, "goblin raw", res.Outcomes[0].DamageRaw, 31)
	eq(t, "ogre raw", res.Outcomes[1].DamageRaw, 15)
	res.ApplyRecommended([]*Target{goblin, ogre})
	eq(t, "goblin HP", goblin.HP, 0) // 10 - 31, clamped at 0
	eq(t, "ogre HP", ogre.HP, 68-15)
	eq(t, "slots left", fmt.Sprint(cs.Slots), "[4 3 1]")
}

func TestFireballUpcastAndMitigation(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(5)))
	// Only two level 3 slots exist at level 5 and no level 4: upcasting fails.
	if _, err := c.Cast(cs, CastRequest{SpellID: "fireball", Slot: 4, Targets: nil, Roller: seq()}); err == nil {
		t.Error("slot 4 should not be available to a level 5 wizard")
	}
	// Give the caster a level 4 slot to test scaling: 8d6 + 1d6 = 9d6.
	cs.Slots = []int{4, 3, 2, 1}
	ogre := monster(t, c, "ogre")
	r := seq(1, 1, 1, 1, 1, 1, 1, 1, 1, 1)
	res := cast(t, c, cs, "fireball", 4, []*Target{ogre}, r, nil)
	done(t, r)
	// 9 dice of 1 = 9; Ogre Dex -1, d20 1 -> 0 < 14 fails: full 9.
	eq(t, "formula", res.Outcomes[0].Damage[0].Formula, "9d6")
	eq(t, "damage", res.Outcomes[0].DamageRaw, 9)

	// Resistance after half-on-success, each rounded down: 31 -> 15 -> 7.
	resist := &Target{Name: "Fire Resistant", Type: "monster", AC: 10, HP: 50, MaxHP: 50, SaveBonus: map[string]int{"dex": 0}, Resist: []string{"fire"}}
	r = seq(6, 5, 4, 3, 2, 1, 6, 4, 20)
	res = cast(t, c, cs, "fireball", 3, []*Target{resist}, r, nil)
	done(t, r)
	res.ApplyRecommended([]*Target{resist})
	eq(t, "raw", res.Outcomes[0].DamageRaw, 15)
	eq(t, "dealt", res.Outcomes[0].DamageDealt, 7)
	eq(t, "HP", resist.HP, 43)

	// Immunity -> 0, vulnerability -> doubled (Skeleton is vulnerable to bludgeoning).
	eq(t, "immune", Mitigate(20, "fire", &Target{Immune: []string{"fire"}}), 0)
	eq(t, "vulnerable", Mitigate(7, "bludgeoning", monster(t, c, "skeleton")), 14)
}

func TestHoldPerson(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	fighterSheet := mustSheet(t, c, fighter3("human"))
	hero := TargetFromSheet("Fighter", fighterSheet, 28)

	// Wisdom save, DC 13. Fighter Wis +1: d20 8 -> 9 < 13 fails -> Paralyzed.
	r := seq(8)
	res := cast(t, c, cs, "hold-person", 2, []*Target{hero}, r, nil)
	done(t, r)
	o := res.Outcomes[0]
	eq(t, "save total", o.Save.Total, 9)
	sameSet(t, "conditions", o.Conditions, []string{"paralyzed"})
	eq(t, "concentration", res.Concentration, true)
	eq(t, "caster concentrating", cs.Concentrating, "hold-person")
	if len(o.Notes) != 1 || !strings.Contains(o.Notes[0], "repeats wis save at end_of_turn") {
		t.Errorf("notes = %v", o.Notes)
	}
	res.ApplyRecommended([]*Target{hero})
	sameSet(t, "target conditions", hero.Conditions, []string{"paralyzed"})

	// d20 12 + 1 = 13 meets the DC: the save succeeds, no condition.
	cs2 := mustCaster(t, mustSheet(t, c, wizard(3)))
	hero2 := TargetFromSheet("Fighter", fighterSheet, 28)
	r = seq(12)
	res = cast(t, c, cs2, "hold-person", 2, []*Target{hero2}, r, nil)
	done(t, r)
	eq(t, "passed", res.Outcomes[0].SavePassed, true)
	eq(t, "conditions", len(res.Outcomes[0].Conditions), 0)
}

func TestHoldPersonRejectsNonHumanoidAndTooManyTargets(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	goblin := monster(t, c, "goblin-warrior") // a Goblin Warrior is a Fey in SRD 5.2, not a Humanoid
	r := seq()
	res := cast(t, c, cs, "hold-person", 2, []*Target{goblin}, r, nil)
	done(t, r) // no dice rolled for an invalid target
	eq(t, "rejected", res.Outcomes[0].Rejected, "invalid target: not humanoid")

	hero := TargetFromSheet("A", mustSheet(t, c, fighter3("human")), 28)
	hero2 := TargetFromSheet("B", mustSheet(t, c, fighter3("human")), 28)
	// Slot 2 allows 1 target; one more per slot level above 2.
	if _, err := c.Cast(cs, CastRequest{SpellID: "hold-person", Slot: 2, Targets: []*Target{hero, hero2}, Roller: seq()}); err == nil {
		t.Error("two targets at slot 2 must be rejected")
	}
	// A level 5 wizard casting at slot 3 may target 2 Humanoids.
	cs5 := mustCaster(t, mustSheet(t, c, wizard(5)))
	r = seq(2, 2)
	res = cast(t, c, cs5, "hold-person", 3, []*Target{hero, hero2}, r, nil)
	done(t, r)
	eq(t, "outcomes", len(res.Outcomes), 2)
}

func TestConcentrationReplaced(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(5)))
	hero := TargetFromSheet("A", mustSheet(t, c, fighter3("human")), 28)
	r := seq(2)
	cast(t, c, cs, "hold-person", 2, []*Target{hero}, r, nil)
	done(t, r)
	res := cast(t, c, cs, "haste", 3, []*Target{hero}, seq(), nil)
	eq(t, "ended", res.EndedConc, "hold-person")
	eq(t, "now", cs.Concentrating, "haste")
	if len(res.Outcomes[0].Buffs) != 3 {
		t.Errorf("haste buffs = %v", res.Outcomes[0].Buffs)
	}
}

func TestCureWounds(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	hero := TargetFromSheet("Fighter", mustSheet(t, c, fighter3("human")), 10)
	// 2d8 + spellcasting modifier (Int +3, via Magic Initiate): 5 + 6 + 3 = 14. HP 10 + 14 = 24.
	r := seq(5, 6)
	res := cast(t, c, cs, "cure-wounds", 1, []*Target{hero}, r, nil)
	done(t, r)
	eq(t, "heal", res.Outcomes[0].Heal, 14)
	res.ApplyRecommended([]*Target{hero})
	eq(t, "HP", hero.HP, 24)

	// Slot 2: 2d8 + 2d8 + 3 with all 1s = 7; healing never exceeds the maximum (28).
	hero.HP = 25
	r = seq(1, 1, 1, 1)
	res = cast(t, c, cs, "cure-wounds", 2, []*Target{hero}, r, nil)
	done(t, r)
	eq(t, "heal", res.Outcomes[0].Heal, 7)
	eq(t, "formula", res.Outcomes[0].Damage[0].Formula, "4d8+3")
	res.ApplyRecommended([]*Target{hero})
	eq(t, "HP capped", hero.HP, 28)
}

func TestShatterConstructDisadvantage(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	golem := &Target{Name: "Construct", Type: "construct", AC: 10, HP: 40, MaxHP: 40, SaveBonus: map[string]int{"con": 1}}
	// 3d8 = 8 + 6 + 2 = 16 rolled first. A Construct saves with Disadvantage:
	// d20s 18 and 5 -> the lower, 5, +1 = 6 < DC 13 -> fails, takes all 16.
	r := seq(8, 6, 2, 18, 5)
	res := cast(t, c, cs, "shatter", 2, []*Target{golem}, r, nil)
	done(t, r)
	eq(t, "adv", res.Outcomes[0].Save.Adv, "disadvantage")
	eq(t, "save total", res.Outcomes[0].Save.Total, 6)
	eq(t, "damage", res.Outcomes[0].DamageRaw, 16)
	sameSet(t, "unhandled", res.Unhandled, []string{"damage_objects"})

	// A non-construct rolls a single d20.
	goblin := monster(t, c, "goblin-warrior")
	r = seq(8, 6, 2, 18)
	res = cast(t, c, cs, "shatter", 2, []*Target{goblin}, r, nil)
	done(t, r)
	eq(t, "adv", res.Outcomes[0].Save.Adv, "") // Con 10 -> +0, 18 >= 13 passes: half of 16 = 8
	eq(t, "damage", res.Outcomes[0].DamageRaw, 8)
}

func TestSacredFlameNoDamageOnSuccess(t *testing.T) {
	c := loadComp(t)
	cs := &Caster{Name: "Cleric", Level: 3, ProfBonus: 2, SpellMod: 3, SpellAttack: 5, SaveDC: 13, Slots: []int{4, 2}}
	goblin := monster(t, c, "goblin-warrior")
	// 1d8 = 5 rolled before the save; goblin Dex +2, d20 11 -> 13 >= 13 succeeds -> no damage.
	r := seq(5, 11)
	res := cast(t, c, cs, "sacred-flame", 0, []*Target{goblin}, r, nil)
	done(t, r)
	eq(t, "passed", res.Outcomes[0].SavePassed, true)
	eq(t, "damage", res.Outcomes[0].DamageRaw, 0)
}

func TestSlotValidation(t *testing.T) {
	c := loadComp(t)
	cs := mustCaster(t, mustSheet(t, c, wizard(3)))
	if _, err := c.Cast(cs, CastRequest{SpellID: "fireball", Slot: 3, Roller: seq()}); err == nil {
		t.Error("a level 3 wizard has no level 3 slot")
	}
	if _, err := c.Cast(cs, CastRequest{SpellID: "hold-person", Slot: 1, Roller: seq()}); err == nil {
		t.Error("hold person cannot be cast with a level 1 slot")
	}
}

func TestSpiritGuardiansDamageTypeChoice(t *testing.T) {
	c := loadComp(t)
	sp := c.Spells["spirit-guardians"]
	d := sp.Effects[0].Damage
	eq(t, "options", strings.Join(d.TypeOptions, "/"), "radiant/necrotic")
	eq(t, "aura area", sp.Effects[0].Area.Shape, "emanation")
}

// Monster actions use the same effects language (Goblin Warrior vs the level 3 Fighter, AC 19).
func TestMonsterAttacks(t *testing.T) {
	c := loadComp(t)
	sheet := mustSheet(t, c, fighter3("human"))
	gob := c.Monsters["goblin-warrior"]

	// d20 14 + 4 = 18 < AC 19: miss.
	hero := TargetFromSheet("Fighter", sheet, 28)
	r := seq(14)
	o, err := MonsterAttack(gob, "Scimitar", hero, "", r)
	if err != nil {
		t.Fatal(err)
	}
	done(t, r)
	eq(t, "hit", o.AttackHit, false)

	// d20 15 + 4 = 19 >= 19: hit; 1d6 + 2 with 4 -> 6. No extra 1d4 without Advantage. 28 - 6 = 22.
	r = seq(15, 4)
	o, _ = MonsterAttack(gob, "Scimitar", hero, "", r)
	done(t, r)
	eq(t, "damage", o.DamageDealt, 6)
	eq(t, "HP", hero.HP, 22)

	// With Advantage: d20s 3 and 15 -> 15 hits; damage 1d6 (4) + 2 + the Advantage rider 1d4 (3) = 9.
	hero = TargetFromSheet("Fighter", sheet, 28)
	r = seq(3, 15, 4, 3)
	o, _ = MonsterAttack(gob, "Scimitar", hero, "advantage", r)
	done(t, r)
	eq(t, "damage", o.DamageDealt, 9)
	eq(t, "HP", hero.HP, 19)

	// Natural 20: crit doubles dice -> 2d6 (3 + 5) + 2 = 10.
	hero = TargetFromSheet("Fighter", sheet, 28)
	r = seq(20, 3, 5)
	o, _ = MonsterAttack(gob, "Scimitar", hero, "", r)
	done(t, r)
	eq(t, "damage", o.DamageDealt, 10)

	// Ogre Greatclub (+6, 2d8 + 4 bludgeoning) vs Skeleton (AC 14, vulnerable to bludgeoning):
	// 10 + 6 = 16 hits; 2d8 = 5 + 6 = 11, + 4 = 15, doubled by vulnerability = 30; HP 13 -> 0.
	skel := monster(t, c, "skeleton")
	r = seq(10, 5, 6)
	o, _ = MonsterAttack(c.Monsters["ogre"], "Greatclub", skel, "", r)
	done(t, r)
	eq(t, "raw", o.DamageRaw, 15)
	eq(t, "dealt", o.DamageDealt, 30)
	eq(t, "HP", skel.HP, 0)
}

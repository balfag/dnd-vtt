package engine

import (
	"fmt"
	"strings"

	"github.com/balfag/dnd-vtt/spikes/srd-data/schema"
)

// Caster is the attacker/caster side of an application.
type Caster struct {
	Name          string
	Level         int // character level (cantrip tiers)
	ProfBonus     int
	SpellMod      int
	SpellAttack   int
	SaveDC        int
	Slots         []int // remaining slots, index 0 = level 1; decremented by Cast
	Flags         map[string]string
	Concentrating string // spell ID currently concentrated on
}

// CasterFromSheet builds a Caster from a derived sheet.
func CasterFromSheet(name string, s *Sheet) (*Caster, error) {
	if s.Spell == nil {
		return nil, fmt.Errorf("%s has no spellcasting", name)
	}
	return &Caster{
		Name: name, Level: s.Level, ProfBonus: s.ProfBonus, SpellMod: s.Spell.Mod,
		SpellAttack: s.Spell.Attack, SaveDC: s.Spell.DC, Slots: append([]int(nil), s.Spell.Slots...), Flags: s.Flags,
	}, nil
}

// Target is a creature an effect is applied to.
type Target struct {
	Name            string
	Type            string
	AC              int
	HP, MaxHP       int
	SaveBonus       map[string]int
	Vulnerable      []string
	Resist          []string
	Immune          []string
	ConditionImmune []string
	Traits          []string
	Conditions      []string
}

// TargetFromMonster makes a Target from a statblock.
func TargetFromMonster(m schema.Monster, name string) *Target {
	t := &Target{
		Name: name, Type: m.Type, AC: m.AC, HP: m.HP, MaxHP: m.HP, SaveBonus: map[string]int{},
		Vulnerable: m.Vulnerable, Resist: m.Resist, Immune: m.Immune, ConditionImmune: m.ConditionImmune,
	}
	for _, a := range Abilities {
		t.SaveBonus[a] = floorDiv(m.Abilities[a]-10, 2)
		if v, ok := m.SaveBonuses[a]; ok {
			t.SaveBonus[a] = v
		}
	}
	return t
}

// TargetFromSheet makes a Target from a player character.
func TargetFromSheet(name string, s *Sheet, hp int) *Target {
	t := &Target{Name: name, Type: "humanoid", AC: s.AC, HP: hp, MaxHP: s.HPMax, SaveBonus: s.Saves, Resist: s.Resist}
	return t
}

// CastRequest is one spell cast.
type CastRequest struct {
	SpellID string
	Slot    int // 0 for cantrips
	Targets []*Target
	// DartTargets assigns each projectile to an index in Targets (Magic Missile).
	DartTargets []int
	DamageType  string // chosen type when the spell offers options
	Roller      Roller
}

// D20 is a d20 roll with advantage handling.
type D20 struct {
	Rolls []int
	Used  int
	Mod   int
	Total int
	Adv   string // "", advantage, disadvantage
}

// Outcome is the result for one target.
type Outcome struct {
	Target      string
	Rejected    string
	Attack      *D20
	AttackHit   bool
	Crit        bool
	Save        *D20
	SaveDC      int
	SavePassed  bool
	Damage      []Roll
	DamageRaw   int
	DamageDealt int
	DamageType  string
	Heal        int
	HPBefore    int
	HPAfter     int
	Conditions  []string
	Buffs       []string
	Notes       []string
}

// CastResult is the recommendation produced by Cast; nothing is applied to
// the targets until ApplyRecommended is called (confirmation step in the UI).
type CastResult struct {
	Spell         string
	Slot          int
	Concentration bool
	EndedConc     string
	Outcomes      []*Outcome
	Unhandled     []string // special handlers needing code
}

type castCtx struct {
	c      *Compendium
	sp     schema.Spell
	caster *Caster
	req    CastRequest
	slot   int
	res    *CastResult
}

// Cast interprets a spell record against targets.
func (c *Compendium) Cast(caster *Caster, req CastRequest) (*CastResult, error) {
	sp, ok := c.Spells[req.SpellID]
	if !ok {
		return nil, fmt.Errorf("unknown spell %q", req.SpellID)
	}
	slot := req.Slot
	if sp.Level == 0 {
		slot = 0
	} else {
		if slot < sp.Level {
			return nil, fmt.Errorf("%s needs a slot of level %d or higher, got %d", sp.Name, sp.Level, slot)
		}
		if slot > len(caster.Slots) || caster.Slots[slot-1] <= 0 {
			return nil, fmt.Errorf("%s has no level %d slot left", caster.Name, slot)
		}
	}
	// Target count: base + extra per slot above.
	max := sp.Targeting.Count
	if sp.Targeting.Extra != nil && sp.Level > 0 {
		max += (slot - sp.Level) * atoi(sp.Targeting.Extra.PerSlot)
	}
	if sp.Targeting.Kind == "creature" || sp.Targeting.Kind == "creatures" {
		if max > 0 && len(req.Targets) > max && sp.ID != "magic-missile" {
			return nil, fmt.Errorf("%s allows %d targets, got %d", sp.Name, max, len(req.Targets))
		}
	}
	if slot > 0 {
		caster.Slots[slot-1]--
	}
	res := &CastResult{Spell: sp.ID, Slot: slot}
	if sp.Duration.Concentration {
		res.Concentration = true
		res.EndedConc = caster.Concentrating
		caster.Concentrating = sp.ID
	}
	cx := &castCtx{c: c, sp: sp, caster: caster, req: req, slot: slot, res: res}
	var shared *Roll // area damage is rolled once for all targets
	for _, t := range req.Targets {
		res.Outcomes = append(res.Outcomes, &Outcome{Target: t.Name, HPBefore: t.HP, HPAfter: t.HP})
	}
	for i, t := range req.Targets {
		o := res.Outcomes[i]
		if f := sp.Targeting.Filter; f != "" && !matchesFilter(f, t) {
			o.Rejected = "invalid target: not " + f
			continue
		}
		cx.effects(sp.Effects, i, t, o, &shared, false)
	}
	return res, nil
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func matchesFilter(f string, t *Target) bool {
	for _, part := range strings.Split(f, ",") {
		switch part {
		case "humanoid":
			if t.Type != "humanoid" {
				return false
			}
		}
	}
	return true
}

func (cx *castCtx) scaled(d schema.Damage, crit bool) (Poly, error) {
	vars := map[string]int{"spellmod": cx.caster.SpellMod, "prof": cx.caster.ProfBonus, "level": cx.caster.Level}
	p, err := ParseExpr(string(d.Dice), vars)
	if err != nil {
		return p, err
	}
	if d.Scale != nil {
		if d.Scale.PerSlot != "" && cx.sp.Level > 0 {
			per, err := ParseExpr(d.Scale.PerSlot, vars)
			if err != nil {
				return p, err
			}
			p.AddN(per, cx.slot-cx.sp.Level)
		}
		if d.Scale.PerTier != "" {
			per, err := ParseExpr(d.Scale.PerTier, vars)
			if err != nil {
				return p, err
			}
			n := 0
			for _, t := range d.Scale.Tiers {
				if cx.caster.Level >= t {
					n++
				}
			}
			p.AddN(per, n)
		}
	}
	return p, nil
}

// effects walks effects for one target. shared caches the area damage roll.
func (cx *castCtx) effects(effs []schema.Effect, idx int, t *Target, o *Outcome, shared **Roll, crit bool) {
	for _, e := range effs {
		switch e.Type {
		case "attack":
			d20 := cx.d20(cx.caster.SpellAttack, "")
			d20.Mod = cx.caster.SpellAttack
			o.Attack = d20
			nat := d20.Used
			o.Crit = nat == 20
			o.AttackHit = nat != 1 && (o.Crit || d20.Total >= t.AC)
			if o.AttackHit {
				cx.effects(e.OnHit, idx, t, o, shared, o.Crit)
			} else if cx.caster.Flags["cantrip.onMissOrSave"] == "set" && cx.sp.Level == 0 {
				o.Notes = append(o.Notes, "Potent Cantrip: half damage on a miss")
				cx.damageOn(findDamage(e.OnHit), t, o, shared, false, true)
			}
		case "save":
			cx.save(e, idx, t, o, shared)
		case "damage":
			if e.Repeat != nil {
				cx.darts(e, t, idx, o)
			} else {
				cx.damageOn(e.Damage, t, o, shared, crit, false)
			}
		case "heal":
			p, err := cx.scaled(*e.Heal, false)
			if err != nil {
				o.Notes = append(o.Notes, err.Error())
				continue
			}
			r := p.Roll(cx.req.Roller, false)
			o.Damage = append(o.Damage, r)
			o.Heal += r.Total
		case "condition":
			cx.addCondition(e.Condition, t, o)
		case "modifier":
			for _, m := range e.Modifiers {
				o.Buffs = append(o.Buffs, m.Target+" "+m.Op+" "+string(m.Value))
			}
		case "move":
			o.Notes = append(o.Notes, fmt.Sprintf("%s %d ft", e.Move.Kind, e.Move.Feet))
		case "special":
			cx.res.Unhandled = append(cx.res.Unhandled, e.Handler)
		}
	}
}

func findDamage(effs []schema.Effect) *schema.Damage {
	for _, e := range effs {
		if e.Type == "damage" && e.Damage != nil {
			return e.Damage
		}
	}
	return nil
}

func (cx *castCtx) d20(mod int, adv string) *D20 {
	r := &D20{Mod: mod, Adv: adv}
	first := cx.req.Roller.Roll(20)
	r.Rolls = []int{first}
	r.Used = first
	if adv != "" {
		second := cx.req.Roller.Roll(20)
		r.Rolls = append(r.Rolls, second)
		if (adv == "advantage") == (second > first) {
			r.Used = second
		}
	}
	r.Total = r.Used + mod
	return r
}

// damageOn rolls (or reuses the shared area roll) and records damage.
func (cx *castCtx) damageOn(d *schema.Damage, t *Target, o *Outcome, shared **Roll, crit, halve bool) {
	if d == nil {
		return
	}
	typ := d.Type
	if typ == "" {
		typ = cx.req.DamageType
		if typ == "" && len(d.TypeOptions) > 0 {
			typ = d.TypeOptions[0]
		}
	}
	var r Roll
	if shared != nil && *shared != nil {
		r = **shared
	} else {
		p, err := cx.scaled(*d, crit)
		if err != nil {
			o.Notes = append(o.Notes, err.Error())
			return
		}
		r = p.Roll(cx.req.Roller, crit)
		if shared != nil && cx.sp.Targeting.Area != nil {
			rr := r
			*shared = &rr
		}
	}
	o.Damage = append(o.Damage, r)
	o.DamageType = typ
	total := r.Total
	if halve {
		total /= 2
	}
	o.DamageRaw += total
}

func (cx *castCtx) darts(e schema.Effect, t *Target, idx int, o *Outcome) {
	vars := map[string]int{}
	n, _ := ParseExpr(string(e.Repeat.Count), vars)
	count := n.Const
	if e.Repeat.Scale != nil && e.Repeat.Scale.PerSlot != "" {
		count += (cx.slot - cx.sp.Level) * atoi(e.Repeat.Scale.PerSlot)
	}
	// Darts are rolled once, on the first target's pass, in dart order.
	if idx != 0 {
		return
	}
	p, err := cx.scaled(*e.Damage, false)
	if err != nil {
		o.Notes = append(o.Notes, err.Error())
		return
	}
	outs := cx.res.Outcomes
	for i := 0; i < count; i++ {
		ti := 0
		if i < len(cx.req.DartTargets) {
			ti = cx.req.DartTargets[i]
		}
		r := p.Roll(cx.req.Roller, false)
		oo := outs[ti]
		oo.Damage = append(oo.Damage, r)
		oo.DamageType = e.Damage.Type
		oo.DamageRaw += r.Total
	}
}

func (cx *castCtx) save(e schema.Effect, idx int, t *Target, o *Outcome, shared **Roll) {
	// Damage is rolled before saves, once per cast, then saves go in target order.
	if e.Damage != nil && *shared == nil {
		p, err := cx.scaled(*e.Damage, false)
		if err == nil {
			r := p.Roll(cx.req.Roller, false)
			*shared = &r
		}
	}
	adv := ""
	auto := false
	if e.Save.Modifiers != nil {
		for _, m := range e.Save.Modifiers {
			if !targetWhen(m.When, t) {
				continue
			}
			switch m.Op {
			case "advantage", "disadvantage":
				adv = m.Op
			case "auto_success":
				auto = true
			}
		}
	}
	dc := cx.caster.SaveDC
	if e.Save.DC != "" {
		p, _ := ParseExpr(string(e.Save.DC), map[string]int{"prof": cx.caster.ProfBonus})
		dc = p.Const
	}
	o.SaveDC = dc
	if auto {
		o.SavePassed = true
		o.Notes = append(o.Notes, "automatic success")
	} else {
		d := cx.d20(t.SaveBonus[e.Save.Ability], adv)
		o.Save = d
		o.SavePassed = d.Total >= dc
	}
	if e.Damage != nil {
		half := e.OnSuccess == "half"
		if o.SavePassed && cx.caster.Flags["cantrip.onMissOrSave"] == "set" && cx.sp.Level == 0 {
			half = true
			o.Notes = append(o.Notes, "Potent Cantrip: half damage on a save")
		}
		if !o.SavePassed || half {
			cx.damageOn(e.Damage, t, o, shared, false, o.SavePassed)
		}
	}
	if !o.SavePassed {
		cx.effects(e.Apply, idx, t, o, shared, false)
	}
}

func targetWhen(w string, t *Target) bool {
	switch {
	case strings.HasPrefix(w, "target.type:"):
		return t.Type == strings.TrimPrefix(w, "target.type:")
	case strings.HasPrefix(w, "target.conditionImmune:"):
		return contains(t.ConditionImmune, strings.TrimPrefix(w, "target.conditionImmune:"))
	case strings.HasPrefix(w, "target.trait:"):
		return contains(t.Traits, strings.TrimPrefix(w, "target.trait:"))
	}
	return true
}

func (cx *castCtx) addCondition(ca *schema.ConditionApply, t *Target, o *Outcome) {
	if contains(t.ConditionImmune, ca.ID) {
		o.Notes = append(o.Notes, "immune to "+ca.ID)
		return
	}
	o.Conditions = append(o.Conditions, ca.ID)
	if ca.RepeatSave != nil {
		o.Notes = append(o.Notes, fmt.Sprintf("repeats %s save at %s", ca.RepeatSave.Ability, ca.RepeatSave.When))
	}
}

// Mitigate applies resistance, immunity and vulnerability (rounded down).
func Mitigate(amount int, typ string, t *Target) int {
	if contains(t.Immune, typ) {
		return 0
	}
	if contains(t.Resist, typ) {
		amount /= 2
	}
	if contains(t.Vulnerable, typ) {
		amount *= 2
	}
	return amount
}

// ApplyRecommended applies a result to the targets (after confirmation):
// damage after mitigation, healing capped at max HP, conditions.
func (r *CastResult) ApplyRecommended(targets []*Target) {
	for i, o := range r.Outcomes {
		t := targets[i]
		o.DamageDealt = Mitigate(o.DamageRaw, o.DamageType, t)
		hp := t.HP - o.DamageDealt + o.Heal
		if hp < 0 {
			hp = 0
		}
		if hp > t.MaxHP {
			hp = t.MaxHP
		}
		t.HP = hp
		o.HPAfter = hp
		t.Conditions = append(t.Conditions, o.Conditions...)
	}
}

// MonsterAttack runs a monster action's attack against a target. advantage:
// "", advantage or disadvantage.
func MonsterAttack(m schema.Monster, action string, t *Target, adv string, r Roller) (*Outcome, error) {
	var act *schema.MonsterAction
	for i := range m.Actions {
		if m.Actions[i].Name == action {
			act = &m.Actions[i]
		}
	}
	if act == nil {
		return nil, fmt.Errorf("%s has no action %q", m.Name, action)
	}
	cx := &castCtx{caster: &Caster{}, req: CastRequest{Roller: r}}
	o := &Outcome{Target: t.Name, HPBefore: t.HP, HPAfter: t.HP}
	for _, e := range act.Effects {
		if e.Type != "attack" {
			continue
		}
		bonus := atoi(string(e.Attack.Bonus))
		d := cx.d20(bonus, adv)
		o.Attack = d
		o.Crit = d.Used == 20
		o.AttackHit = d.Used != 1 && (o.Crit || d.Total >= t.AC)
		if !o.AttackHit {
			continue
		}
		for _, h := range e.OnHit {
			if h.When == "attack:advantage" && adv != "advantage" {
				continue
			}
			p, err := ParseExpr(string(h.Damage.Dice), nil)
			if err != nil {
				return nil, err
			}
			roll := p.Roll(r, o.Crit)
			o.Damage = append(o.Damage, roll)
			o.DamageType = h.Damage.Type
			o.DamageRaw += roll.Total
		}
	}
	o.DamageDealt = Mitigate(o.DamageRaw, o.DamageType, t)
	t.HP -= o.DamageDealt
	if t.HP < 0 {
		t.HP = 0
	}
	o.HPAfter = t.HP
	return o, nil
}

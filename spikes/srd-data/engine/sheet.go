package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/balfag/dnd-vtt/spikes/srd-data/schema"
)

// Abilities in rules order.
var Abilities = []string{"str", "dex", "con", "int", "wis", "cha"}

// skillAbility is the SRD skill table. It is rules data that belongs in a
// data file (see REPORT.md); kept in code in the spike.
var skillAbility = map[string]string{
	"acrobatics": "dex", "animal-handling": "wis", "arcana": "int", "athletics": "str",
	"deception": "cha", "history": "int", "insight": "wis", "intimidation": "cha",
	"investigation": "int", "medicine": "wis", "nature": "int", "perception": "wis",
	"performance": "cha", "persuasion": "cha", "religion": "int", "sleight-of-hand": "dex",
	"stealth": "dex", "survival": "wis",
}

// Character holds only player-entered data; everything else is derived.
// Abilities are final scores (background increases already applied).
type Character struct {
	Name       string
	Species    string
	Background string
	Class      string
	Level      int
	Abilities  map[string]int
	// Choices maps a choice ID to the picked values (class skills, lineage,
	// subclass, fighting style, "asi-4" as "asi:wis+2" or "feat:<id>").
	Choices  map[string][]string
	Equipped []string // item IDs
	Buffs    []string // spell IDs whose modifier effects are active (Mage Armor)
}

// AttackLine is a weapon attack as shown on the sheet.
type AttackLine struct {
	Item      string
	Bonus     int
	Damage    string
	DamageTwo string // versatile, two-handed
	DamageTyp string
}

// SpellStats are the spellcasting numbers.
type SpellStats struct {
	Ability  string
	DC       int
	Attack   int
	Mod      int
	Cantrips int
	Prepared int
	Slots    []int
}

// Sheet is the derived character sheet.
type Sheet struct {
	Level         int
	ProfBonus     int
	Abilities     map[string]int
	Mods          map[string]int
	Saves         map[string]int
	SaveProf      map[string]bool
	Skills        map[string]int
	SkillProf     map[string]string // proficient | expertise
	Passive       map[string]int
	AC            int
	HPMax         int
	Initiative    int
	Speed         int
	Senses        map[string]int
	Attacks       []AttackLine
	Spell         *SpellStats
	Flags         map[string]string // "target" -> "advantage" | "disadvantage" | "flag" | "set"
	Values        map[string]int    // numeric value of "set" modifiers (crit.range)
	Feats         []string
	GrantedSpells []string
	Features      []string
	Uses          map[string]int
	Handlers      []string // special handlers this character relies on (not interpreted)
	Conflicts     []string // e.g. duplicate proficiencies
	Resist        []string
}

type modEntry struct {
	schema.Modifier
	source string
}

type builder struct {
	c           *Compendium
	ch          Character
	s           *Sheet
	mods        []modEntry
	vars        map[string]int
	armor       *schema.ArmorData
	shield      bool
	err         error
	seenFeats   map[string]bool
	profs       map[string]string // "skill:athletics" -> level
	weaponProfs map[string]bool
	cols        map[string]int
}

// BuildSheet computes the derived sheet for ch.
func (c *Compendium) BuildSheet(ch Character) (*Sheet, error) {
	cl, ok := c.Classes[ch.Class]
	if !ok {
		return nil, fmt.Errorf("unknown class %q", ch.Class)
	}
	sp, ok := c.Species[ch.Species]
	if !ok {
		return nil, fmt.Errorf("unknown species %q", ch.Species)
	}
	bg, ok := c.Backgrounds[ch.Background]
	if !ok {
		return nil, fmt.Errorf("unknown background %q", ch.Background)
	}
	if ch.Level < 1 || ch.Level > 5 {
		return nil, fmt.Errorf("level %d outside the spike range 1..5", ch.Level)
	}

	b := &builder{c: c, ch: ch, seenFeats: map[string]bool{}, profs: map[string]string{}, weaponProfs: map[string]bool{}, cols: map[string]int{}}
	b.s = &Sheet{
		Level: ch.Level, Abilities: map[string]int{}, Mods: map[string]int{}, Saves: map[string]int{},
		SaveProf: map[string]bool{}, Skills: map[string]int{}, SkillProf: map[string]string{},
		Passive: map[string]int{}, Senses: map[string]int{}, Flags: map[string]string{}, Values: map[string]int{}, Uses: map[string]int{},
	}
	s := b.s
	s.ProfBonus = 2 + (ch.Level-1)/4

	// Ability scores; "asi:" picks apply before modifiers are computed.
	for _, a := range Abilities {
		s.Abilities[a] = ch.Abilities[a]
	}
	for id, picks := range ch.Choices {
		if !strings.HasPrefix(id, "asi-") {
			continue
		}
		for _, p := range picks {
			if rest, ok := strings.CutPrefix(p, "asi:"); ok {
				for _, part := range strings.Split(rest, ",") {
					ab, n, ok := strings.Cut(part, "+")
					if !ok {
						return nil, fmt.Errorf("bad asi pick %q", p)
					}
					v, _ := strconv.Atoi(n)
					s.Abilities[ab] += v
				}
			}
		}
	}
	for _, a := range Abilities {
		s.Mods[a] = floorDiv(s.Abilities[a]-10, 2)
	}

	// Class table columns for this level.
	var row *schema.ClassLevel
	for i := range cl.Levels {
		if cl.Levels[i].Level == ch.Level {
			row = &cl.Levels[i]
		}
	}
	if row == nil {
		return nil, fmt.Errorf("class %q has no row for level %d", cl.ID, ch.Level)
	}
	for k, v := range row.Columns {
		b.cols[k] = v
	}
	b.vars = map[string]int{"prof": s.ProfBonus, "level": ch.Level}
	for _, a := range Abilities {
		b.vars["mod."+a] = s.Mods[a]
	}
	for k, v := range b.cols {
		b.vars["col."+k] = v
	}

	// Class proficiencies, saving throws.
	for _, sv := range cl.SavingThrows {
		s.SaveProf[sv] = true
	}
	b.applyProfs(cl.Proficiencies)
	b.applyChoice(cl.SkillChoice, "class:"+cl.ID)

	// Class features up to the level, plus level-table choices.
	for _, lv := range cl.Levels {
		if lv.Level > ch.Level {
			continue
		}
		for _, fid := range lv.Features {
			for _, f := range cl.Features {
				if f.ID == fid {
					b.applyFeature(f, "class:"+cl.ID)
				}
			}
		}
		for _, chc := range lv.Choices {
			b.applyChoice(chc, "level:"+strconv.Itoa(lv.Level))
		}
	}
	// Subclass chosen at a level choice.
	if picks := ch.Choices[cl.ID+"-subclass"]; len(picks) == 1 {
		sc, ok := c.Subclasses[picks[0]]
		if !ok || sc.Class != cl.ID {
			return nil, fmt.Errorf("subclass %q is not a %s subclass", picks[0], cl.ID)
		}
		for _, f := range sc.Features {
			if f.Level <= ch.Level {
				b.applyFeature(f, "subclass:"+sc.ID)
			}
		}
	}

	// Species and background.
	s.Speed = sp.Speed
	for _, f := range sp.Traits {
		b.applyFeature(f, "species:"+sp.ID)
	}
	b.applyProfs(bg.Proficiencies)
	for _, chc := range bg.Choices {
		b.applyChoice(chc, "background:"+bg.ID)
	}
	b.grantFeat(bg.Feat, bg.FeatPresets)

	// Equipment.
	for _, id := range ch.Equipped {
		it, ok := c.Items[id]
		if !ok {
			return nil, fmt.Errorf("unknown item %q", id)
		}
		if it.Kind == "armor" && it.Armor != nil {
			if it.Armor.StrMin > s.Abilities["str"] {
				b.s.Conflicts = append(b.s.Conflicts, fmt.Sprintf("%s needs Str %d: Speed reduced by 10 ft", it.Name, it.Armor.StrMin))
			}
			b.armor = it.Armor
		}
		if it.Kind == "shield" {
			b.shield = true
		}
	}
	// Active buffs (spell effects of type modifier).
	for _, id := range ch.Buffs {
		sp, ok := c.Spells[id]
		if !ok {
			return nil, fmt.Errorf("unknown buff spell %q", id)
		}
		for _, e := range sp.Effects {
			if e.Type == "modifier" {
				b.collect(e.Modifiers, "buff:"+id)
			}
		}
	}
	if b.err != nil {
		return nil, b.err
	}

	b.derive(cl, row)
	return s, b.err
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func (b *builder) fail(format string, args ...any) {
	if b.err == nil {
		b.err = fmt.Errorf(format, args...)
	}
}

func (b *builder) applyProfs(ps []schema.Proficiency) {
	for _, p := range ps {
		switch p.Kind {
		case "skill":
			key := "skill:" + p.Key
			if p.Level == "expertise" {
				if b.profs[key] == "" {
					b.s.Conflicts = append(b.s.Conflicts, "expertise in "+p.Key+" without proficiency")
				}
				b.profs[key] = "expertise"
			} else if b.profs[key] != "" {
				b.s.Conflicts = append(b.s.Conflicts, "duplicate proficiency "+p.Key)
			} else {
				b.profs[key] = "proficient"
			}
		case "save":
			b.s.SaveProf[p.Key] = true
		case "weapon":
			b.weaponProfs[p.Key] = true
		}
	}
}

func (b *builder) collect(ms []schema.Modifier, src string) {
	for _, m := range ms {
		b.mods = append(b.mods, modEntry{m, src})
	}
}

func (b *builder) applyFeature(f schema.Feature, src string) {
	b.s.Features = append(b.s.Features, f.ID)
	if f.Uses != nil {
		if p, err := ParseExpr(string(f.Uses.Max), b.vars); err == nil {
			b.s.Uses[f.ID] = p.Const
		} else {
			b.fail("feature %s uses: %v", f.ID, err)
		}
	}
	b.applyEffects(f.Effects, src, "")
}

func (b *builder) grantFeat(id string, presets map[string][]string) {
	if b.seenFeats[id] {
		b.s.Conflicts = append(b.s.Conflicts, "feat "+id+" granted twice")
		return
	}
	f, ok := b.c.Feats[id]
	if !ok {
		b.fail("unknown feat %q", id)
		return
	}
	b.seenFeats[id] = true
	b.s.Feats = append(b.s.Feats, id)
	for k, v := range presets {
		if _, set := b.ch.Choices[k]; !set {
			if b.ch.Choices == nil {
				b.ch.Choices = map[string][]string{}
			}
			b.ch.Choices[k] = v
		}
	}
	b.applyEffects(f.Effects, "feat:"+id, "")
}

// applyChoice validates picks for c and applies Then/ThenBy.
func (b *builder) applyChoice(c schema.Choice, src string) {
	want := c.Count
	if c.CountExpr != "" {
		p, err := ParseExpr(string(c.CountExpr), b.vars)
		if err != nil {
			b.fail("choice %s: %v", c.ID, err)
			return
		}
		want = p.Const
	}
	picks := b.ch.Choices[c.ID]
	if strings.HasPrefix(c.FromKind, "asi_or_feat") {
		if len(picks) != want {
			b.fail("choice %s (%s): need %d picks, have %d", c.ID, src, want, len(picks))
			return
		}
		for _, p := range picks {
			if id, ok := strings.CutPrefix(p, "feat:"); ok {
				b.grantFeat(id, nil)
			}
		}
		return
	}
	if len(picks) != want {
		b.fail("choice %s (%s): need %d picks, have %d %v", c.ID, src, want, len(picks), picks)
		return
	}
	for _, p := range picks {
		if len(c.From) > 0 && !contains(c.From, p) {
			b.fail("choice %s: %q is not one of %v", c.ID, p, c.From)
			return
		}
		if strings.HasPrefix(c.FromKind, "feat:") {
			cat := strings.TrimPrefix(c.FromKind, "feat:")
			if f, ok := b.c.Feats[p]; !ok || f.Category != cat {
				b.fail("choice %s: %q is not a %s feat", c.ID, p, cat)
				return
			}
		}
		if c.FromKind == "skill" {
			if _, ok := skillAbility[p]; !ok {
				b.fail("choice %s: %q is not a skill", c.ID, p)
				return
			}
		}
		b.applyEffects(c.Then, src, p)
		b.applyEffects(c.ThenBy[p], src, p)
	}
}

// applyEffects interprets the sheet-relevant effect types; others are ignored
// here (they matter when casting). pick substitutes "$choice".
func (b *builder) applyEffects(effs []schema.Effect, src, pick string) {
	for _, e := range effs {
		if pick != "" {
			e = substitute(e, pick)
		}
		if e.When != "" && !b.whenLevel(e.When) {
			continue
		}
		switch e.Type {
		case "modifier":
			b.collect(e.Modifiers, src)
		case "proficiency":
			b.applyProfs(e.Proficiencies)
		case "choice":
			b.applyChoice(*e.Choice, src)
		case "grant":
			switch {
			case strings.HasPrefix(e.Grant, "feat:"):
				b.grantFeat(strings.TrimPrefix(e.Grant, "feat:"), nil)
			case strings.HasPrefix(e.Grant, "spell:"):
				id, rider, _ := strings.Cut(strings.TrimPrefix(e.Grant, "spell:"), ":")
				b.s.GrantedSpells = append(b.s.GrantedSpells, id)
				if rider != "" {
					b.s.Uses[rider+":"+id] = 1
				}
			}
		case "special":
			b.s.Handlers = append(b.s.Handlers, e.Handler)
		}
	}
}

func (b *builder) whenLevel(w string) bool {
	if rest, ok := strings.CutPrefix(w, "level>="); ok {
		n, _ := strconv.Atoi(rest)
		return b.ch.Level >= n
	}
	return true
}

// substitute replaces "$choice" in an effect by round-tripping through JSON.
func substitute(e schema.Effect, pick string) schema.Effect {
	raw, _ := json.Marshal(e)
	raw = []byte(strings.ReplaceAll(string(raw), "$choice", pick))
	var out schema.Effect
	_ = json.Unmarshal(raw, &out)
	return out
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// condOK evaluates a modifier condition against the character's gear.
func (b *builder) condOK(w string) bool {
	switch w {
	case "":
		return true
	case "armor:none":
		return b.armor == nil
	case "armor:worn":
		return b.armor != nil
	}
	return true
}

func (b *builder) derive(cl schema.Class, row *schema.ClassLevel) {
	s := b.s
	// Saves.
	for _, a := range Abilities {
		v := s.Mods[a]
		if s.SaveProf[a] {
			v += s.ProfBonus
		}
		s.Saves[a] = v
	}
	// Skills.
	for sk, ab := range skillAbility {
		v := s.Mods[ab]
		switch b.profs["skill:"+sk] {
		case "proficient":
			v += s.ProfBonus
			s.SkillProf[sk] = "proficient"
		case "expertise":
			v += 2 * s.ProfBonus
			s.SkillProf[sk] = "expertise"
		}
		s.Skills[sk] = v
	}
	// Generic modifiers.
	acBase := []int{10 + s.Mods["dex"]}
	if b.armor != nil {
		dex := s.Mods["dex"]
		switch b.armor.DexMode {
		case "none":
			dex = 0
		case "max2":
			if dex > 2 {
				dex = 2
			}
		}
		acBase = []int{b.armor.Base + dex}
	}
	acBonus := 0
	s.Speed = b.speedStart()
	speedMult := 1
	hpPerLevel, hpFlat := 0, 0
	initBonus := 0
	for _, m := range b.mods {
		if !b.condOK(m.When) {
			continue
		}
		val := 0
		if m.Value != "" {
			if p, err := ParseExpr(string(m.Value), b.vars); err == nil {
				val = p.Const
			} else if c := m.Value[0]; c == '@' || (c >= '0' && c <= '9') {
				b.fail("modifier %s: %v", m.Target, err)
				continue
			} // otherwise the value is symbolic ("half", "magic-missile") and val stays 0
		}
		switch {
		case m.Target == "ac.base" && m.Op == "set":
			acBase = append(acBase, val)
		case m.Target == "ac.bonus" && m.Op == "add":
			acBonus += val
		case m.Target == "speed" && m.Op == "set":
			s.Speed = val
		case m.Target == "speed.mult" && m.Op == "mult":
			speedMult *= val
		case m.Target == "hp.max.perLevel":
			hpPerLevel += val
		case m.Target == "hp.max.flat":
			hpFlat += val
		case m.Target == "initiative" && m.Op == "add":
			initBonus += val
		case strings.HasPrefix(m.Target, "sense.") && m.Op == "set":
			if val > s.Senses[strings.TrimPrefix(m.Target, "sense.")] {
				s.Senses[strings.TrimPrefix(m.Target, "sense.")] = val
			}
		case strings.HasPrefix(m.Target, "resist.") && m.Op == "flag":
			s.Resist = append(s.Resist, strings.TrimPrefix(m.Target, "resist."))
		default:
			s.Flags[m.Target] = m.Op
			s.Values[m.Target] = val
		}
	}
	best := acBase[0]
	for _, v := range acBase {
		if v > best {
			best = v
		}
	}
	s.AC = best + acBonus
	if b.shield {
		s.AC += 2
	}
	s.Initiative = s.Mods["dex"] + initBonus
	_ = speedMult
	s.Speed *= speedMult

	// HP: max die at level 1, fixed average (die/2+1) after.
	hp := 0
	for l := 1; l <= s.Level; l++ {
		gain := cl.HitDie/2 + 1
		if l == 1 {
			gain = cl.HitDie
		}
		gain += s.Mods["con"] + hpPerLevel
		if gain < 1 {
			gain = 1
		}
		hp += gain
	}
	s.HPMax = hp + hpFlat

	// Passive scores.
	s.Passive["perception"] = 10 + s.Skills["perception"]
	s.Passive["insight"] = 10 + s.Skills["insight"]
	s.Passive["investigation"] = 10 + s.Skills["investigation"]

	// Weapon attacks.
	for _, id := range b.ch.Equipped {
		it := b.c.Items[id]
		if it.Kind != "weapon" || it.Weapon == nil {
			continue
		}
		w := it.Weapon
		ab := "str"
		if w.Ranged {
			ab = "dex"
		} else if contains(w.Properties, "finesse") && s.Mods["dex"] > s.Mods["str"] {
			ab = "dex"
		}
		bonus := s.Mods[ab]
		if b.weaponProfs[w.Category] {
			bonus += s.ProfBonus
		}
		line := AttackLine{Item: it.Name, Bonus: bonus, DamageTyp: w.Damage.Type, Damage: withMod(string(w.Damage.Dice), s.Mods[ab])}
		if w.Versatile != "" {
			line.DamageTwo = withMod(w.Versatile, s.Mods[ab])
		}
		s.Attacks = append(s.Attacks, line)
	}

	// Spellcasting.
	if cl.Spellcasting != nil {
		ab := cl.Spellcasting.Ability
		s.Spell = &SpellStats{
			Ability: ab, Mod: s.Mods[ab], DC: 8 + s.ProfBonus + s.Mods[ab], Attack: s.ProfBonus + s.Mods[ab],
			Cantrips: row.Cantrips, Prepared: row.Prepared, Slots: append([]int(nil), row.Slots...),
		}
	}
}

func (b *builder) speedStart() int { return b.c.Species[b.ch.Species].Speed }

func withMod(dice string, mod int) string {
	switch {
	case mod > 0:
		return fmt.Sprintf("%s + %d", dice, mod)
	case mod < 0:
		return fmt.Sprintf("%s - %d", dice, -mod)
	}
	return dice
}

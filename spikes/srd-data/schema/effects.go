// Package schema is the draft content schema for SRD 5.2 and homebrew records.
//
// Every record carries Meta (schemaVersion, id, source). The effects language
// (this file) is shared by spells, class features, feats, species traits,
// items and monster actions, so the engine is written once.
package schema

// SchemaVersion is the current version of the content schema.
const SchemaVersion = 1

// Source values for Meta.Source.
const (
	SourceSRD      = "srd-5.2.1"
	SourceHomebrew = "homebrew"
)

// Meta is common to all records. Names are looked up by ID in locale packs
// (see REPORT.md, localization); Name here is the English fallback.
type Meta struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        string `json:"source"`
}

// Expr is a formula string evaluated by the engine: integers, dice (NdM),
// + and -, and variables: @prof, @level, @mod.<ability>, @spellmod, @slot.
// Example: "8 + @prof + @spellmod", "2d8 + @spellmod".
type Expr string

// Scale describes how a value grows. PerSlot is added once per spell slot
// level above the spell's level; Tiers/PerTier add once per tier reached by
// character level (cantrips: 5, 11, 17).
type Scale struct {
	PerSlot string `json:"perSlot,omitempty"`
	Tiers   []int  `json:"tiers,omitempty"`
	PerTier string `json:"perTier,omitempty"`
}

// Damage is a damage roll. TypeOptions lets the caster pick the type at cast
// time (Spirit Guardians: radiant or necrotic).
type Damage struct {
	Dice        Expr     `json:"dice"`
	Type        string   `json:"type,omitempty"`
	TypeOptions []string `json:"typeOptions,omitempty"`
	Scale       *Scale   `json:"scale,omitempty"`
}

// Modifier changes a derived value. Target examples: "ac.base", "ac.bonus",
// "speed", "speed.mult", "save.dex", "skill.perception", "initiative",
// "hp.max.flat", "hp.max.perLevel", "attacks.against", "crit.range".
// Op: add | set | mult | advantage | disadvantage | flag.
// When is a condition string such as "armor:none", "armor:worn",
// "weapon:melee", "target.type:construct".
type Modifier struct {
	Target string `json:"target"`
	Op     string `json:"op"`
	Value  Expr   `json:"value,omitempty"`
	When   string `json:"when,omitempty"`
}

// Proficiency grants proficiency. Kind: skill | save | weapon | armor | tool.
// Key may be "$choice" - replaced by the value picked for the enclosing Choice.
type Proficiency struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Level string `json:"level,omitempty"` // "" or "proficient", or "expertise"
}

// Choice is a decision made by the player (or the level-up wizard). Picked
// values are stored in the character under the choice ID. Then effects are
// applied once per picked value with "$choice" substituted. FromKind lets From
// be resolved from content ("feat:origin", "spell:cantrip:wizard").
type Choice struct {
	ID       string   `json:"id"`
	Count    int      `json:"count"`
	From     []string `json:"from,omitempty"`
	FromKind string   `json:"fromKind,omitempty"`
	Then     []Effect `json:"then,omitempty"`
	// ThenBy maps a picked value to extra effects (lineage -> benefits).
	ThenBy map[string][]Effect `json:"thenBy,omitempty"`
}

// Area shapes: sphere, cube, cone, line, cylinder, emanation.
type Area struct {
	Shape        string `json:"shape"`
	Size         int    `json:"size"`                   // radius / edge / length in feet
	Width        int    `json:"width,omitempty"`        // lines
	Origin       string `json:"origin"`                 // "point" (chosen within range) or "self"
	ChosenExempt bool   `json:"chosenExempt,omitempty"` // caster may exclude creatures
}

// Duration. Kind: instant | round | minute | hour | until_long_rest.
type Duration struct {
	Kind          string `json:"kind"`
	Amount        int    `json:"amount,omitempty"`
	Concentration bool   `json:"concentration,omitempty"`
}

// SaveSpec is a saving throw. DC defaults to the caster's spell save DC.
type SaveSpec struct {
	Ability     string     `json:"ability"`
	DC          Expr       `json:"dc,omitempty"`
	IgnoreCover bool       `json:"ignoreCover,omitempty"`
	Modifiers   []Modifier `json:"modifiers,omitempty"` // e.g. disadvantage when target.type:construct
	When        string     `json:"when,omitempty"`      // "end_of_turn" for repeat saves
}

// ConditionApply imposes a condition. Until: duration | end_of_next_turn |
// start_of_next_turn. RepeatSave lets the target retry. OnExpire effects run
// when the condition ends by time (Sleep: incapacitated -> unconscious).
type ConditionApply struct {
	ID         string    `json:"id"`
	Until      string    `json:"until,omitempty"`
	RepeatSave *SaveSpec `json:"repeatSave,omitempty"`
	OnExpire   []Effect  `json:"onExpire,omitempty"`
	EndsOn     []string  `json:"endsOn,omitempty"` // damage | shaken
}

// Move moves a creature: push | teleport.
type Move struct {
	Kind string `json:"kind"`
	Feet int    `json:"feet"`
}

// Uses is limited-use resource. Max is an Expr or a class-table column ref
// ("@col.secondWindUses"). Recharge: short_rest | long_rest | dawn.
type Uses struct {
	Max      Expr   `json:"max"`
	Recharge string `json:"recharge"`
}

// Effect is one step of the effects language. Type selects which fields are used:
//
//	attack      Attack, OnHit
//	save        Save, Damage (+OnSuccess), Apply (on failure)
//	damage      Damage (auto-hit; Repeat for several darts)
//	heal        Heal
//	condition   Condition
//	modifier    Modifiers, Duration, Until (buff on the target)
//	proficiency Proficiencies
//	choice      Choice
//	move        Move
//	aura        Area, Trigger, Save, Damage, Modifiers (persistent zone)
//	grant       Grant (extra action etc.)
//	special     Handler (code escape hatch; Reason says why)
type Effect struct {
	Type          string          `json:"type"`
	Attack        *AttackSpec     `json:"attack,omitempty"`
	OnHit         []Effect        `json:"onHit,omitempty"`
	Save          *SaveSpec       `json:"save,omitempty"`
	OnSuccess     string          `json:"onSuccess,omitempty"` // half | none (damage on a passed save)
	Apply         []Effect        `json:"apply,omitempty"`
	Damage        *Damage         `json:"damage,omitempty"`
	Repeat        *Repeat         `json:"repeat,omitempty"`
	Heal          *Damage         `json:"heal,omitempty"`
	Condition     *ConditionApply `json:"condition,omitempty"`
	Modifiers     []Modifier      `json:"modifiers,omitempty"`
	Proficiencies []Proficiency   `json:"proficiencies,omitempty"`
	Choice        *Choice         `json:"choice,omitempty"`
	Move          *Move           `json:"move,omitempty"`
	Area          *Area           `json:"area,omitempty"`
	Trigger       string          `json:"trigger,omitempty"` // enter | end_turn | start_in_area
	Once          string          `json:"once,omitempty"`    // per_turn
	Grant         string          `json:"grant,omitempty"`
	Duration      *Duration       `json:"duration,omitempty"`
	Until         string          `json:"until,omitempty"`
	Handler       string          `json:"handler,omitempty"`
	OnEnd         []Effect        `json:"onEnd,omitempty"` // runs when a timed modifier/condition ends (Haste lethargy)
	Reason        string          `json:"reason,omitempty"`
	When          string          `json:"when,omitempty"`
}

// AttackSpec is an attack roll. Kind: spell | weapon. Bonus is an Expr for
// monsters ("+4"); spells and weapons compute it from the attacker.
type AttackSpec struct {
	Kind   string `json:"kind"`
	Ranged bool   `json:"ranged,omitempty"`
	Bonus  Expr   `json:"bonus,omitempty"`
}

// Repeat makes an effect happen N times (Magic Missile darts). Count scales.
type Repeat struct {
	Count Expr   `json:"count"`
	Scale *Scale `json:"scale,omitempty"`
}

// Feature is a named ability of a class, subclass, species, background or feat.
type Feature struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Level       int      `json:"level,omitempty"`
	Description string   `json:"description,omitempty"`
	Effects     []Effect `json:"effects,omitempty"`
	Uses        *Uses    `json:"uses,omitempty"`
}

package schema

// Spell is a spell record. Description is attributed SRD text (shortened);
// mechanics live only in Effects/Targeting/Scaling.
type Spell struct {
	Meta
	Level       int        `json:"level"`
	School      string     `json:"school"`
	Ritual      bool       `json:"ritual,omitempty"`
	Classes     []string   `json:"classes"`
	CastingTime CastTime   `json:"castingTime"`
	Range       Range      `json:"range"`
	Components  Components `json:"components"`
	Duration    Duration   `json:"duration"`
	Targeting   Targeting  `json:"targeting"`
	Effects     []Effect   `json:"effects"`
	Description string     `json:"description"`
}

// CastTime. Unit: action | bonus_action | reaction | minute.
type CastTime struct {
	Unit    string `json:"unit"`
	Trigger string `json:"trigger,omitempty"`
}

// Range. Kind: self | touch | distance.
type Range struct {
	Kind string `json:"kind"`
	Feet int    `json:"feet,omitempty"`
}

type Components struct {
	V        bool   `json:"v,omitempty"`
	S        bool   `json:"s,omitempty"`
	M        bool   `json:"m,omitempty"`
	Material string `json:"material,omitempty"`
}

// Targeting. Kind: creature | creatures | point | self | area_self.
// Filter e.g. "humanoid", "willing", "unarmored". Extra (per slot above) adds
// targets (Hold Person, Bless).
type Targeting struct {
	Kind   string `json:"kind"`
	Count  int    `json:"count,omitempty"`
	Filter string `json:"filter,omitempty"`
	Extra  *Scale `json:"extra,omitempty"`
	Area   *Area  `json:"area,omitempty"`
}

// Class holds the level table, features and spellcasting of a class.
type Class struct {
	Meta
	HitDie         int           `json:"hitDie"`
	PrimaryAbility []string      `json:"primaryAbility"`
	SavingThrows   []string      `json:"savingThrows"`
	Proficiencies  []Proficiency `json:"proficiencies"`
	SkillChoice    Choice        `json:"skillChoice"`
	Spellcasting   *Spellcasting `json:"spellcasting,omitempty"`
	Levels         []ClassLevel  `json:"levels"`
	Features       []Feature     `json:"features"`
	Subclasses     []string      `json:"subclasses"`
	Multiclass     Multiclass    `json:"multiclass"`
}

// Spellcasting. Preparation: "spellbook" or "class_list".
type Spellcasting struct {
	Ability     string `json:"ability"`
	Preparation string `json:"preparation"`
	Ritual      bool   `json:"ritual,omitempty"`
}

// ClassLevel is one row of the class table. Columns holds class-specific
// numbers (secondWindUses). Choices are decisions required on reaching the level.
type ClassLevel struct {
	Level    int            `json:"level"`
	Features []string       `json:"features,omitempty"`
	Columns  map[string]int `json:"columns,omitempty"`
	Cantrips int            `json:"cantrips,omitempty"`
	Prepared int            `json:"prepared,omitempty"`
	Slots    []int          `json:"slots,omitempty"` // index 0 = level 1
	Choices  []Choice       `json:"choices,omitempty"`
}

// Multiclass: prerequisites (any listed ability >= min, or all) and the
// proficiencies gained when entering the class as a non-first class.
type Multiclass struct {
	RequireAny    []AbilityMin  `json:"requireAny,omitempty"`
	RequireAll    []AbilityMin  `json:"requireAll,omitempty"`
	Proficiencies []Proficiency `json:"proficiencies,omitempty"`
}

type AbilityMin struct {
	Ability string `json:"ability"`
	Min     int    `json:"min"`
}

// Subclass belongs to a class; Features carry their own Level.
type Subclass struct {
	Meta
	Class    string    `json:"class"`
	Features []Feature `json:"features"`
}

// Species (2024 "race"). Traits may contain Choices (Elven Lineage, Keen Senses).
type Species struct {
	Meta
	CreatureType string    `json:"creatureType"`
	Size         string    `json:"size"`
	Speed        int       `json:"speed"`
	Traits       []Feature `json:"traits"`
}

// Background grants (2024) an ability score increase among AbilityOptions,
// an origin feat, skill/tool proficiencies.
type Background struct {
	Meta
	AbilityOptions []string `json:"abilityOptions"`
	Feat           string   `json:"feat"`
	// FeatPresets fixes some of the granted feat's choices (Magic Initiate: Cleric).
	FeatPresets   map[string][]string `json:"featPresets,omitempty"`
	Proficiencies []Proficiency       `json:"proficiencies"`
	Choices       []Choice            `json:"choices,omitempty"`
	Description   string              `json:"description,omitempty"`
}

// Feat. Category: origin | general | fighting_style | epic_boon.
type Feat struct {
	Meta
	Category     string   `json:"category"`
	Prerequisite string   `json:"prerequisite,omitempty"`
	Repeatable   bool     `json:"repeatable,omitempty"`
	Description  string   `json:"description"`
	Effects      []Effect `json:"effects,omitempty"`
}

// Item. Kind: weapon | armor | shield | consumable | gear.
type Item struct {
	Meta
	Kind        string      `json:"kind"`
	CostGP      float64     `json:"costGp"`
	WeightLb    float64     `json:"weightLb"`
	Weapon      *WeaponData `json:"weapon,omitempty"`
	Armor       *ArmorData  `json:"armor,omitempty"`
	Effects     []Effect    `json:"effects,omitempty"` // consumables, magic properties
	CastTime    string      `json:"castTime,omitempty"`
	Description string      `json:"description,omitempty"`
}

// WeaponData. Category: simple | martial. Properties: finesse, versatile,
// ammunition, two_handed, light, heavy, reach, thrown, loading.
type WeaponData struct {
	Category   string   `json:"category"`
	Ranged     bool     `json:"ranged"`
	Damage     Damage   `json:"damage"`
	Versatile  string   `json:"versatile,omitempty"`
	Properties []string `json:"properties"`
	RangeFt    []int    `json:"rangeFt,omitempty"` // normal, long
	Mastery    string   `json:"mastery,omitempty"`
}

// ArmorData. Category: light | medium | heavy | shield. DexMode: full | max2 | none.
// For a shield Base is the AC bonus.
type ArmorData struct {
	Category      string `json:"category"`
	Base          int    `json:"base"`
	DexMode       string `json:"dexMode,omitempty"`
	StrMin        int    `json:"strMin,omitempty"`
	StealthDisadv bool   `json:"stealthDisadv,omitempty"`
}

// Monster is a statblock. NPCs/PCs can reuse the same shape (ADR-0009).
type Monster struct {
	Meta
	Size            string          `json:"size"`
	Type            string          `json:"type"`
	Alignment       string          `json:"alignment"`
	AC              int             `json:"ac"`
	ACSource        string          `json:"acSource,omitempty"`
	HP              int             `json:"hp"`
	HPDice          string          `json:"hpDice"`
	Speed           map[string]int  `json:"speed"`
	Abilities       map[string]int  `json:"abilities"`
	SaveBonuses     map[string]int  `json:"saveBonuses,omitempty"`
	SkillBonuses    map[string]int  `json:"skillBonuses,omitempty"`
	Vulnerable      []string        `json:"vulnerable,omitempty"`
	Resist          []string        `json:"resist,omitempty"`
	Immune          []string        `json:"immune,omitempty"`
	ConditionImmune []string        `json:"conditionImmune,omitempty"`
	Senses          map[string]int  `json:"senses,omitempty"`
	Languages       string          `json:"languages"`
	CR              string          `json:"cr"`
	XP              int             `json:"xp"`
	ProfBonus       int             `json:"profBonus"`
	Traits          []Feature       `json:"traits,omitempty"`
	Actions         []MonsterAction `json:"actions"`
	BonusActions    []MonsterAction `json:"bonusActions,omitempty"`
}

// MonsterAction. Description is the SRD wording; Effects carry the mechanics.
type MonsterAction struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Effects     []Effect `json:"effects,omitempty"`
}

// Condition. Effects are declarative where possible.
type Condition struct {
	Meta
	Description string     `json:"description"`
	Includes    []string   `json:"includes,omitempty"` // other conditions implied
	Modifiers   []Modifier `json:"modifiers,omitempty"`
}

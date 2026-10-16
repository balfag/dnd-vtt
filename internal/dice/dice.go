// Package dice parses dice formulas (docs/systems/vtt.md, «Формулы бросков»)
// and rolls them with a full per-die breakdown.
package dice

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Limits of the formula grammar.
const (
	maxFormulaLen = 200
	maxTerms      = 20
	maxTotalDice  = 200
	maxConstant   = 1_000_000
	maxCount      = 100
	minSides      = 2
	maxSides      = 1000
	maxExplosions = 100
)

// Roller returns a uniformly random integer in [1, sides].
type Roller interface {
	Roll(sides int) int
}

type cryptoRoller struct{}

func (cryptoRoller) Roll(sides int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(sides)))
	if err != nil {
		// The OS random source failed: nothing sensible can be rolled after
		// this, and no caller can recover. The only panic of the package.
		panic(fmt.Sprintf("dice: crypto/rand failed: %v", err))
	}
	return int(n.Int64()) + 1
}

// CryptoRoller rolls with crypto/rand.
func CryptoRoller() Roller { return cryptoRoller{} }

// ParseError is a syntax or limit error at rune position Pos (0-based) of the input.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("dice: %s at %d", e.Msg, e.Pos)
}

// keepKind is the kind of a keep/drop modifier.
type keepKind int

const (
	keepNone keepKind = iota
	keepHighest
	keepLowest
	dropHighest
	dropLowest
)

// compareOp is the comparison of a reroll condition.
type compareOp int

const (
	cmpEqual compareOp = iota
	cmpLess
	cmpLessEq
	cmpGreater
	cmpGreaterEq
)

// matches reports whether a die showing v satisfies "v op n".
func (op compareOp) matches(v, n int) bool {
	switch op {
	case cmpEqual:
		return v == n
	case cmpLess:
		return v < n
	case cmpLessEq:
		return v <= n
	case cmpGreater:
		return v > n
	case cmpGreaterEq:
		return v >= n
	}
	return false
}

func (op compareOp) String() string {
	switch op {
	case cmpLess:
		return "<"
	case cmpLessEq:
		return "<="
	case cmpGreater:
		return ">"
	case cmpGreaterEq:
		return ">="
	}
	return ""
}

// group is a dice term: NdM with modifiers.
type group struct {
	count, sides int

	keep  keepKind
	keepN int

	reroll   bool
	rerollOp compareOp
	rerollN  int

	explode bool
}

// term is one summand: a constant or a dice group.
type term struct {
	sign     int // +1 or -1
	constant int
	group    *group // nil for a constant
}

// Formula is a parsed dice formula. The zero value is not valid.
type Formula struct {
	terms []term
}

// Result is a rolled formula. JSON tags are for the future roll log.
type Result struct {
	Formula string       `json:"formula"` // canonical
	Total   int          `json:"total"`
	Terms   []TermResult `json:"terms"`
}

// TermResult is one rolled summand.
type TermResult struct {
	Sign  int        `json:"sign"`           // +1 or -1
	Value int        `json:"value"`          // the constant, or the sum of kept dice
	Dice  *DiceGroup `json:"dice,omitempty"` // nil for a constant
}

// DiceGroup is a rolled dice term.
type DiceGroup struct {
	Count int   `json:"count"` // dice before explosions
	Sides int   `json:"sides"`
	Dice  []Die `json:"dice"`
}

// Die is one rolled die.
type Die struct {
	Value        int  `json:"value"`
	Dropped      bool `json:"dropped,omitempty"`      // removed by kh/kl/dh/dl
	RerolledFrom int  `json:"rerolledFrom,omitempty"` // original value if rerolled, 0 otherwise
	Exploded     bool `json:"exploded,omitempty"`     // added by an explosion
}

package dice

import (
	"strconv"
	"strings"
)

// Parse parses s. Errors are *ParseError.
func Parse(s string) (Formula, error) {
	rs := []rune(s)
	if len(rs) > maxFormulaLen {
		return Formula{}, &ParseError{Pos: maxFormulaLen, Msg: "formula is too long"}
	}
	p := &parser{rs: rs}
	if err := p.formula(); err != nil {
		return Formula{}, err
	}
	return Formula{terms: p.terms}, nil
}

type parser struct {
	rs         []rune
	pos        int
	terms      []term
	totalDice  int
	overflowed bool // the last number did not fit the counting limit
}

func (p *parser) errAt(pos int, msg string) error {
	return &ParseError{Pos: pos, Msg: msg}
}

func (p *parser) eof() bool { return p.pos >= len(p.rs) }

func (p *parser) peek() rune { return p.rs[p.pos] }

// peekAt returns the rune at pos+offset lower-cased, or 0 past the end.
func (p *parser) peekAt(offset int) rune {
	if i := p.pos + offset; i < len(p.rs) {
		return lower(p.rs[i])
	}
	return 0
}

func lower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func (p *parser) skipSpace() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

// number reads digits. A value above 1<<31 stops growing and sets p.overflowed,
// so the caller's range check rejects it without an int overflow.
func (p *parser) number() int {
	const limit = 1 << 31
	p.overflowed = false
	n := 0
	for !p.eof() && isDigit(p.peek()) {
		if n < limit {
			n = n*10 + int(p.peek()-'0')
		}
		if n >= limit {
			p.overflowed = true
		}
		p.pos++
	}
	return n
}

func (p *parser) formula() error {
	p.skipSpace()
	sign := 1
	if !p.eof() && (p.peek() == '+' || p.peek() == '-') {
		if p.peek() == '-' {
			sign = -1
		}
		p.pos++
	}
	for {
		if err := p.term(sign); err != nil {
			return err
		}
		p.skipSpace()
		if p.eof() {
			return nil
		}
		switch p.peek() {
		case '+':
			sign = 1
		case '-':
			sign = -1
		default:
			return p.errAt(p.pos, "unexpected character "+strconv.QuoteRune(p.peek()))
		}
		p.pos++
	}
}

func (p *parser) term(sign int) error {
	p.skipSpace()
	start := p.pos
	if len(p.terms) >= maxTerms {
		return p.errAt(start, "too many terms")
	}
	if p.eof() {
		return p.errAt(start, "expected a number or dice")
	}

	switch {
	case isDigit(p.peek()):
		n := p.number()
		overflowed := p.overflowed
		p.skipSpace()
		if p.peekAt(0) == 'd' {
			return p.dice(sign, start, n, overflowed)
		}
		if overflowed || n > maxConstant {
			return p.errAt(start, "number is too large")
		}
		p.terms = append(p.terms, term{sign: sign, constant: n})
		return nil
	case lower(p.peek()) == 'd':
		return p.dice(sign, start, 1, false)
	}
	return p.errAt(start, "expected a number or dice")
}

// dice parses "d" sides {modifier}; the count has been read already.
func (p *parser) dice(sign, start, count int, countOverflowed bool) error {
	if countOverflowed || count < 1 || count > maxCount {
		return p.errAt(start, "dice count must be 1..100")
	}
	p.pos++ // the "d"
	p.skipSpace()

	g := &group{count: count}
	switch {
	case p.eof():
		return p.errAt(p.pos, "expected the number of sides")
	case p.peek() == '%':
		g.sides = 100
		p.pos++
	case isDigit(p.peek()):
		sidesPos := p.pos
		n := p.number()
		if p.overflowed || n < minSides || n > maxSides {
			return p.errAt(sidesPos, "sides must be 2..1000")
		}
		g.sides = n
	default:
		return p.errAt(p.pos, "expected the number of sides")
	}

	p.totalDice += count
	if p.totalDice > maxTotalDice {
		return p.errAt(start, "too many dice in the formula")
	}
	if err := p.modifiers(g); err != nil {
		return err
	}
	p.terms = append(p.terms, term{sign: sign, group: g})
	return nil
}

func (p *parser) modifiers(g *group) error {
	for {
		p.skipSpace()
		if p.eof() {
			return nil
		}
		at := p.pos
		switch c := lower(p.peek()); c {
		case 'k', 'd':
			if err := p.keepModifier(g, at); err != nil {
				return err
			}
		case 'r':
			if err := p.rerollModifier(g, at); err != nil {
				return err
			}
		case '!':
			if g.explode {
				return p.errAt(at, "only one explode modifier is allowed")
			}
			g.explode = true
			p.pos++
		default:
			return nil
		}
	}
}

func (p *parser) keepModifier(g *group, at int) error {
	var kind keepKind
	switch [2]rune{p.peekAt(0), p.peekAt(1)} {
	case [2]rune{'k', 'h'}:
		kind = keepHighest
	case [2]rune{'k', 'l'}:
		kind = keepLowest
	case [2]rune{'d', 'h'}:
		kind = dropHighest
	case [2]rune{'d', 'l'}:
		kind = dropLowest
	default:
		return p.errAt(at, "unknown modifier")
	}
	if g.keep != keepNone {
		return p.errAt(at, "only one keep or drop modifier is allowed")
	}
	p.pos += 2
	p.skipSpace()
	n := 1
	if !p.eof() && isDigit(p.peek()) {
		n = p.number()
		if p.overflowed {
			n = 0
		}
	}
	if n < 1 || n > g.count {
		return p.errAt(at, "keep or drop count must be 1..dice count")
	}
	g.keep, g.keepN = kind, n
	return nil
}

func (p *parser) rerollModifier(g *group, at int) error {
	if g.reroll {
		return p.errAt(at, "only one reroll modifier is allowed")
	}
	p.pos++ // the "r"
	p.skipSpace()

	op := cmpEqual
	switch {
	case !p.eof() && p.peek() == '<':
		op = cmpLess
		p.pos++
		if !p.eof() && p.peek() == '=' {
			op = cmpLessEq
			p.pos++
		}
	case !p.eof() && p.peek() == '>':
		op = cmpGreater
		p.pos++
		if !p.eof() && p.peek() == '=' {
			op = cmpGreaterEq
			p.pos++
		}
	}
	p.skipSpace()
	if p.eof() || !isDigit(p.peek()) {
		return p.errAt(p.pos, "expected a number after the reroll condition")
	}
	n := p.number()
	if p.overflowed || n < 1 || n > g.sides {
		return p.errAt(at, "reroll number must be 1..sides")
	}

	covered := 0
	for v := 1; v <= g.sides; v++ {
		if op.matches(v, n) {
			covered++
		}
	}
	if covered == g.sides {
		return p.errAt(at, "reroll condition covers every side")
	}
	g.reroll, g.rerollOp, g.rerollN = true, op, n
	return nil
}

// String returns the canonical form: lower case, no spaces, explicit count,
// d% as d100, keep counts explicit (e.g. "d20+ 5" -> "1d20+5", "2D20KH" -> "2d20kh1").
func (f Formula) String() string {
	var b strings.Builder
	for i, t := range f.terms {
		switch {
		case t.sign < 0:
			b.WriteByte('-')
		case i > 0:
			b.WriteByte('+')
		}
		if t.group == nil {
			b.WriteString(strconv.Itoa(t.constant))
			continue
		}
		g := t.group
		b.WriteString(strconv.Itoa(g.count))
		b.WriteByte('d')
		b.WriteString(strconv.Itoa(g.sides))
		switch g.keep {
		case keepHighest:
			b.WriteString("kh")
		case keepLowest:
			b.WriteString("kl")
		case dropHighest:
			b.WriteString("dh")
		case dropLowest:
			b.WriteString("dl")
		}
		if g.keep != keepNone {
			b.WriteString(strconv.Itoa(g.keepN))
		}
		if g.reroll {
			b.WriteByte('r')
			b.WriteString(g.rerollOp.String())
			b.WriteString(strconv.Itoa(g.rerollN))
		}
		if g.explode {
			b.WriteByte('!')
		}
	}
	return b.String()
}

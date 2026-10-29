// Package engine is the prototype interpreter for the content schema.
package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// Roller rolls one die with the given number of sides.
type Roller interface {
	Roll(sides int) int
}

// SeqRoller returns a fixed sequence of values; used for deterministic tests.
// Out-of-range values or running out of values set Err instead of panicking.
type SeqRoller struct {
	Values []int
	pos    int
	Err    error
}

func (r *SeqRoller) Roll(sides int) int {
	if r.pos >= len(r.Values) {
		if r.Err == nil {
			r.Err = fmt.Errorf("dice sequence exhausted after %d values (d%d requested)", r.pos, sides)
		}
		return 1
	}
	v := r.Values[r.pos]
	r.pos++
	if v < 1 || v > sides {
		if r.Err == nil {
			r.Err = fmt.Errorf("value %d at position %d is not a valid d%d result", v, r.pos-1, sides)
		}
		return 1
	}
	return v
}

// Remaining is the number of unused values.
func (r *SeqRoller) Remaining() int { return len(r.Values) - r.pos }

// Die is a group of N dice with Sides faces and a Sign.
type Die struct {
	N     int
	Sides int
	Sign  int
}

// Poly is a parsed formula: Const + sum of dice groups.
type Poly struct {
	Const int
	Dice  []Die
}

// ParseExpr parses a formula. vars supplies @name values.
func ParseExpr(s string, vars map[string]int) (Poly, error) {
	var p Poly
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return p, fmt.Errorf("empty expression")
	}
	sign := 1
	i := 0
	for i < len(s) {
		switch s[i] {
		case '+':
			sign = 1
			i++
			continue
		case '-':
			sign = -1
			i++
			continue
		}
		j := i
		if s[i] == '@' {
			j++
			for j < len(s) && s[j] != '+' && s[j] != '-' {
				j++
			}
			name := s[i+1 : j]
			v, ok := vars[name]
			if !ok {
				return p, fmt.Errorf("unknown variable @%s in %q", name, s)
			}
			p.Const += sign * v
		} else {
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j == i {
				return p, fmt.Errorf("unexpected %q in %q", s[i:], s)
			}
			n, _ := strconv.Atoi(s[i:j])
			if j < len(s) && s[j] == 'd' {
				k := j + 1
				for k < len(s) && s[k] >= '0' && s[k] <= '9' {
					k++
				}
				sides, err := strconv.Atoi(s[j+1 : k])
				if err != nil {
					return p, fmt.Errorf("bad dice in %q", s)
				}
				p.addDie(Die{N: n, Sides: sides, Sign: sign})
				j = k
			} else {
				p.Const += sign * n
			}
		}
		sign = 1
		i = j
	}
	return p, nil
}

func (p *Poly) addDie(d Die) {
	for i := range p.Dice {
		if p.Dice[i].Sides == d.Sides && p.Dice[i].Sign == d.Sign {
			p.Dice[i].N += d.N
			return
		}
	}
	p.Dice = append(p.Dice, d)
}

// AddN adds o to p n times (scaling: +1d6 per slot level).
func (p *Poly) AddN(o Poly, n int) {
	p.Const += o.Const * n
	for _, d := range o.Dice {
		d.N *= n
		p.addDie(d)
	}
}

func (p Poly) String() string {
	var parts []string
	for _, d := range p.Dice {
		parts = append(parts, fmt.Sprintf("%dd%d", d.N, d.Sides))
	}
	s := strings.Join(parts, "+")
	if p.Const != 0 || s == "" {
		if p.Const >= 0 && s != "" {
			s += "+"
		}
		s += strconv.Itoa(p.Const)
	}
	return s
}

// DieRoll is one rolled die.
type DieRoll struct {
	Sides int
	Value int
}

// Roll is the outcome of rolling a Poly.
type Roll struct {
	Formula string
	Dice    []DieRoll
	Total   int
}

// Roll rolls p; crit doubles the number of dice (not the constant).
func (p Poly) Roll(r Roller, crit bool) Roll {
	out := Roll{Formula: p.String(), Total: p.Const}
	if crit {
		out.Formula += " (crit)"
	}
	for _, d := range p.Dice {
		n := d.N
		if crit {
			n *= 2
		}
		for k := 0; k < n; k++ {
			v := r.Roll(d.Sides)
			out.Dice = append(out.Dice, DieRoll{Sides: d.Sides, Value: v})
			out.Total += d.Sign * v
		}
	}
	return out
}

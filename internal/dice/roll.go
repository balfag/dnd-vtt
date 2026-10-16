package dice

import "sort"

// Roll rolls the formula with r.
func (f Formula) Roll(r Roller) Result {
	res := Result{Formula: f.String(), Terms: make([]TermResult, 0, len(f.terms))}
	for _, t := range f.terms {
		tr := TermResult{Sign: t.sign, Value: t.constant}
		if t.group != nil {
			dg := t.group.roll(r)
			tr.Dice = &dg
			tr.Value = 0
			for _, d := range dg.Dice {
				if !d.Dropped {
					tr.Value += d.Value
				}
			}
		}
		res.Terms = append(res.Terms, tr)
		res.Total += t.sign * tr.Value
	}
	return res
}

// roll rolls one group: dice, one reroll pass, explosions, then keep or drop.
func (g *group) roll(r Roller) DiceGroup {
	dice := make([]Die, g.count)
	for i := range dice {
		dice[i].Value = r.Roll(g.sides)
	}

	if g.reroll {
		for i := range dice {
			if g.rerollOp.matches(dice[i].Value, g.rerollN) {
				dice[i].RerolledFrom = dice[i].Value
				dice[i].Value = r.Roll(g.sides)
			}
		}
	}

	if g.explode {
		added := 0
		// The slice grows while it is walked: new dice may explode too.
		for i := 0; i < len(dice) && added < maxExplosions; i++ {
			if dice[i].Value == g.sides {
				dice = append(dice, Die{Value: r.Roll(g.sides), Exploded: true})
				added++
			}
		}
	}

	g.selectDice(dice)
	return DiceGroup{Count: g.count, Sides: g.sides, Dice: dice}
}

// selectDice marks the dice removed by the keep or drop modifier. Among equal
// values the die with the larger index is dropped first.
func (g *group) selectDice(dice []Die) {
	if g.keep == keepNone {
		return
	}
	// kh N drops the (total - N) lowest, kl N the (total - N) highest.
	drop, lowest := g.keepN, false
	switch g.keep {
	case keepHighest:
		drop, lowest = len(dice)-g.keepN, true
	case keepLowest:
		drop, lowest = len(dice)-g.keepN, false
	case dropHighest:
		lowest = false
	case dropLowest:
		lowest = true
	}

	order := make([]int, len(dice))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		va, vb := dice[order[a]].Value, dice[order[b]].Value
		if va != vb {
			if lowest {
				return va < vb
			}
			return va > vb
		}
		return order[a] > order[b]
	})
	for _, i := range order[:drop] {
		dice[i].Dropped = true
	}
}

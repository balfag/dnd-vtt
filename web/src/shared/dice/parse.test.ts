import { describe, expect, it } from 'vitest'
import formulas from '../../../../internal/dice/testdata/formulas.json'
import { isValidFormula } from './parse'

describe('isValidFormula', () => {
  it('has the shared cases', () => {
    expect(formulas.length).toBeGreaterThanOrEqual(60)
  })

  it.each(formulas.map((c) => [JSON.stringify(c.formula.slice(0, 40)), c] as const))(
    '%s',
    (_name, c) => {
      expect(isValidFormula(c.formula)).toBe(c.ok)
    },
  )
})

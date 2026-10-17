import { describe, expect, it } from 'vitest'
import { parseInitiative } from './initiative'

describe('parseInitiative', () => {
  it.each([
    ['14', 14],
    [' 7 ', 7],
    ['0', 0],
    ['-0', 0],
    ['-5', -5],
    ['−5', -5],
    ['99', 99],
    ['-99', -99],
    ['05', 5],
  ])('reads %j as %j', (text, want) => {
    expect(parseInitiative(text)).toBe(want)
  })

  it('reads an empty field as null, not as 0', () => {
    expect(parseInitiative('')).toBeNull()
    expect(parseInitiative('   ')).toBeNull()
  })

  it.each(['100', '-100', '1.5', '1e1', 'abc', '--3', '+3', '1 2', '123'])('refuses %j', (text) => {
    expect(parseInitiative(text)).toBeUndefined()
  })
})

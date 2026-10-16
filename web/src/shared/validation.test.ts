import { describe, expect, it } from 'vitest'
import { isValidDisplayName, isValidLogin, isValidPassword, normalizeLogin } from './validation'

describe('normalizeLogin', () => {
  it('trims and lowercases', () => {
    expect(normalizeLogin(' Anna ')).toBe('anna')
  })
})

describe('isValidLogin', () => {
  it.each(['abc', 'a'.repeat(32), 'a.b_c-1', ' Anna '])('accepts %j', (login) => {
    expect(isValidLogin(login)).toBe(true)
  })

  it.each(['ab', 'a'.repeat(33), 'анна', 'a b', 'a@b', ''])('rejects %j', (login) => {
    expect(isValidLogin(login)).toBe(false)
  })
})

describe('isValidPassword', () => {
  it.each(['12345678', 'a'.repeat(128), 'пароль12', 'пароль!!'])('accepts %j', (password) => {
    expect(isValidPassword(password)).toBe(true)
  })

  it.each(['1234567', 'a'.repeat(129), ''])('rejects a password of %d code points', (password) => {
    expect(isValidPassword(password)).toBe(false)
  })

  it('counts code points, not UTF-16 units', () => {
    expect(isValidPassword('😀'.repeat(8))).toBe(true)
    expect(isValidPassword('😀'.repeat(129))).toBe(false)
  })
})

describe('isValidDisplayName', () => {
  it.each(['A', 'Анна', 'x'.repeat(64), ' Anna '])('accepts %j', (name) => {
    expect(isValidDisplayName(name)).toBe(true)
  })

  it.each(['', '   ', 'x'.repeat(65)])('rejects %j', (name) => {
    expect(isValidDisplayName(name)).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import { validIcon } from './icon'

describe('validIcon', () => {
  it('accepts nothing, an emoji and up to eight code points', () => {
    expect(validIcon('')).toBe(true)
    expect(validIcon('🍺')).toBe(true)
    expect(validIcon('👨‍👩‍👧')).toBe(true)
    expect(validIcon('12345678')).toBe(true)
  })

  it('rejects more than eight code points', () => {
    expect(validIcon('123456789')).toBe(false)
    expect(validIcon('🍺🍺🍺🍺🍺🍺🍺🍺🍺')).toBe(false)
  })

  it('rejects control characters', () => {
    expect(validIcon('a\nb')).toBe(false)
    expect(validIcon('\u0000')).toBe(false)
    expect(validIcon('\u007f')).toBe(false)
  })
})

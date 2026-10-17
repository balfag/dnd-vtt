import { describe, expect, it } from 'vitest'
import { contrast, readableColor } from './playerColor'

// The server's player palette (internal/game playerColors).
const palette = [
  '#e53935',
  '#1e88e5',
  '#43a047',
  '#fb8c00',
  '#8e24aa',
  '#00acc1',
  '#fdd835',
  '#6d4c41',
]

describe('contrast', () => {
  it('is 21 for black on white and 1 for equal colors', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5)
    expect(contrast('#23303a', '#23303a')).toBeCloseTo(1, 5)
  })

  it('does not depend on the order', () => {
    expect(contrast('#ece7db', '#23303a')).toBe(contrast('#23303a', '#ece7db'))
  })
})

describe('readableColor', () => {
  it.each(palette)('%s is readable on slate', (hex) => {
    expect(contrast(readableColor(hex), '#23303a')).toBeGreaterThanOrEqual(4.5)
  })

  it('lightens a dark color', () => {
    expect(readableColor('#6d4c41')).not.toBe('#6d4c41')
  })

  it('keeps a color that is readable already', () => {
    expect(readableColor('#fdd835')).toBe('#fdd835')
  })

  it('leaves chalk alone', () => {
    expect(readableColor('#ECE7DB')).toBe('#ece7db')
  })

  it('returns lowercase #rrggbb, the same every time', () => {
    const got = readableColor('#1E88E5')
    expect(got).toMatch(/^#[0-9a-f]{6}$/)
    expect(readableColor('#1E88E5')).toBe(got)
  })
})

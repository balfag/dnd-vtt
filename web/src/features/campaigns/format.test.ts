import { describe, expect, it } from 'vitest'
import { formatDate, formatMoment } from './format'

const utc = 'UTC'
const now = new Date('2026-10-16T18:00:00Z')

describe('formatMoment', () => {
  it('shows only the time for today', () => {
    expect(formatMoment(Date.parse('2026-10-16T07:05:00Z'), 'en', now, utc)).toMatch(/^0?7:05/)
    expect(formatMoment(Date.parse('2026-10-16T19:40:00Z'), 'ru', now, utc)).toBe('19:40')
  })

  it('shows the date and the time for another day', () => {
    const text = formatMoment(Date.parse('2026-10-12T19:40:00Z'), 'ru', now, utc)
    expect(text).toContain('12')
    expect(text).toContain('октября')
    expect(text).toContain('19:40')
  })

  it('tells today from yesterday around midnight in the given zone', () => {
    const lateEvening = Date.parse('2026-10-15T23:30:00Z')
    expect(formatMoment(lateEvening, 'ru', new Date('2026-10-16T00:10:00Z'), utc)).toContain(
      'октября',
    )
    expect(formatMoment(lateEvening, 'ru', new Date('2026-10-15T23:50:00Z'), utc)).toBe('23:30')
  })
})

describe('formatDate', () => {
  it('omits the year of this year', () => {
    const text = formatDate(Date.parse('2026-10-12T10:00:00Z'), 'ru', now, utc)
    expect(text).toBe('12 октября')
  })

  it('adds the year of another year', () => {
    const text = formatDate(Date.parse('2025-09-03T10:00:00Z'), 'ru', now, utc)
    expect(text).toContain('2025')
    expect(text).toContain('сентября')
  })
})

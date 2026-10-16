import { describe, expect, it } from 'vitest'
import { reconnectDelay } from './backoff'

describe('reconnectDelay', () => {
  it('doubles up to the cap with a middle random', () => {
    const got = [1, 2, 3, 4, 5, 6].map((n) => reconnectDelay(n, () => 0.5))
    expect(got).toEqual([500, 1000, 2000, 4000, 8000, 10000])
  })

  it('stays at the cap for late attempts', () => {
    expect(reconnectDelay(10, () => 0.5)).toBe(10000)
  })

  it('scales by the lower bound of the jitter', () => {
    expect(reconnectDelay(1, () => 0)).toBe(400)
  })

  it('rounds near the upper bound of the jitter', () => {
    const got = reconnectDelay(5, () => 0.999999)
    expect(Number.isInteger(got)).toBe(true)
    expect(got).toBeCloseTo(9600, -1)
  })

  it('treats attempt below 1 as 1', () => {
    expect(reconnectDelay(0, () => 0.5)).toBe(500)
    expect(reconnectDelay(-3, () => 0.5)).toBe(500)
  })
})

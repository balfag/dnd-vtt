import { describe, expect, it } from 'vitest'
import { simplify, thin } from './simplify'

describe('simplify', () => {
  it('reduces a straight line of 100 points to its ends', () => {
    const line = Array.from({ length: 100 }, (_, i) => ({ x: i * 3, y: i * 2 }))
    expect(simplify(line, 1.5)).toEqual([line[0], line[99]])
  })

  it('keeps the corners of a polyline', () => {
    const corner = [
      { x: 0, y: 0 },
      { x: 5, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 5 },
      { x: 10, y: 10 },
    ]
    expect(simplify(corner, 1)).toEqual([corner[0], corner[2], corner[4]])
  })

  it('drops a wiggle within epsilon and keeps one beyond it', () => {
    const bump = [
      { x: 0, y: 0 },
      { x: 5, y: 2 },
      { x: 10, y: 0 },
    ]
    expect(simplify(bump, 3)).toHaveLength(2)
    expect(simplify(bump, 1)).toHaveLength(3)
  })

  it('changes nothing with epsilon 0', () => {
    const line = Array.from({ length: 10 }, (_, i) => ({ x: i, y: 0 }))
    expect(simplify(line, 0)).toEqual(line)
  })

  it('survives a stroke of many points', () => {
    const wave = Array.from({ length: 20000 }, (_, i) => ({ x: i, y: Math.sin(i / 50) * 100 }))
    const out = simplify(wave, 1.5)
    expect(out.length).toBeLessThan(wave.length)
    expect(out[0]).toEqual(wave[0])
  })
})

describe('thin', () => {
  it('keeps short lines as they are', () => {
    const pts = [
      { x: 0, y: 0 },
      { x: 1, y: 1 },
    ]
    expect(thin(pts, 5)).toEqual(pts)
  })

  it('cuts a long line to the limit and keeps both ends', () => {
    const pts = Array.from({ length: 5000 }, (_, i) => ({ x: i, y: 0 }))
    const out = thin(pts, 2000)
    expect(out).toHaveLength(2000)
    expect(out[0]).toEqual(pts[0])
    expect(out[1999]).toEqual(pts[4999])
  })
})

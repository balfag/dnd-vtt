import { describe, expect, it } from 'vitest'
import { tileRects } from './tiles'

describe('tileRects', () => {
  it('splits 20000x3000 at 4096 into 5x1 tiles covering everything without overlap', () => {
    const tiles = tileRects(20000, 3000, 4096)
    expect(tiles).toHaveLength(5)
    expect(tiles.every((t) => t.y === 0 && t.h === 3000)).toBe(true)
    expect(tiles.map((t) => t.x)).toEqual([0, 4096, 8192, 12288, 16384])
    expect(tiles.reduce((sum, t) => sum + t.w * t.h, 0)).toBe(20000 * 3000)
    for (let i = 1; i < tiles.length; i++) {
      const prev = tiles[i - 1]
      const cur = tiles[i]
      if (prev === undefined || cur === undefined) throw new Error('missing tile')
      expect(cur.x).toBe(prev.x + prev.w)
    }
  })

  it('returns one tile for an image smaller than max', () => {
    expect(tileRects(800, 600, 4096)).toEqual([{ x: 0, y: 0, w: 800, h: 600 }])
  })

  it('goes row by row and covers the area of a big image exactly', () => {
    const tiles = tileRects(9000, 5000, 4096)
    expect(tiles).toHaveLength(6)
    expect(tiles[3]).toEqual({ x: 0, y: 4096, w: 4096, h: 904 })
    expect(tiles.reduce((sum, t) => sum + t.w * t.h, 0)).toBe(9000 * 5000)
  })

  it('does not split an image exactly max wide', () => {
    expect(tileRects(4096, 4096, 4096)).toHaveLength(1)
  })
})

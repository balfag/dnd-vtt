import { describe, expect, it } from 'vitest'
import type { Grid } from '../../../shared/protocol'
import type { Point } from './camera'
import { cellAt, cellCenter, cellPolygon, cellsInRect, type Cell } from './grid'

function gridOf(type: Grid['type']): Grid {
  return {
    type,
    sizePx: 70,
    unitFt: 5,
    offsetX: 13.5,
    offsetY: -21,
    color: '#9aa8b3',
    visible: true,
  }
}

const types: Grid['type'][] = ['square', 'hexPointy', 'hexFlat']

function cells100(): Cell[] {
  const out: Cell[] = []
  for (let row = -5; row < 5; row++) {
    for (let col = -5; col < 5; col++) out.push({ col, row })
  }
  return out
}

function dist(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y)
}

function insidePolygon(poly: Point[], p: Point): boolean {
  let inside = false
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[i]
    const b = poly[j]
    if (a === undefined || b === undefined) continue
    if (a.y > p.y !== b.y > p.y && p.x < ((b.x - a.x) * (p.y - a.y)) / (b.y - a.y) + a.x) {
      inside = !inside
    }
  }
  return inside
}

function sharedVertices(a: Point[], b: Point[]): number {
  return a.filter((p) => b.some((q) => dist(p, q) < 1e-9)).length
}

describe.each(types)('grid %s', (type) => {
  const g = gridOf(type)

  it('cellAt(cellCenter(c)) is c for 100 cells around zero', () => {
    for (const c of cells100()) {
      expect(cellAt(g, cellCenter(g, c))).toEqual(c)
    }
  })

  it('cellAt finds the cell for points near its vertices', () => {
    for (const c of cells100()) {
      const m = cellCenter(g, c)
      for (const v of cellPolygon(g, c)) {
        const p = { x: m.x + (v.x - m.x) * 0.95, y: m.y + (v.y - m.y) * 0.95 }
        expect(cellAt(g, p)).toEqual(c)
      }
    }
  })

  it('cellPolygon has 4 or 6 vertices and contains the centre', () => {
    const want = type === 'square' ? 4 : 6
    for (const c of cells100()) {
      const poly = cellPolygon(g, c)
      expect(poly).toHaveLength(want)
      expect(insidePolygon(poly, cellCenter(g, c))).toBe(true)
    }
  })

  it('cellsInRect lists every cell whose centre is in the rect', () => {
    const r = { x: -200, y: -150, w: 520, h: 400 }
    const got = cellsInRect(g, r)
    for (const c of cells100()) {
      const m = cellCenter(g, c)
      if (m.x >= r.x && m.x <= r.x + r.w && m.y >= r.y && m.y <= r.y + r.h) {
        expect(got).toContainEqual(c)
      }
    }
  })

  it('cellsInRect does not list cells far from the rect', () => {
    const r = { x: 0, y: 0, w: 100, h: 100 }
    for (const c of cellsInRect(g, r)) {
      const m = cellCenter(g, c)
      expect(m.x).toBeGreaterThan(r.x - g.sizePx * 1.2)
      expect(m.x).toBeLessThan(r.x + r.w + g.sizePx * 1.2)
      expect(m.y).toBeGreaterThan(r.y - g.sizePx * 1.2)
      expect(m.y).toBeLessThan(r.y + r.h + g.sizePx * 1.2)
    }
  })
})

describe.each(['hexPointy', 'hexFlat'] as const)('%s neighbours', (type) => {
  const g = gridOf(type)

  it('every inner hex has six neighbours that share an edge', () => {
    for (let row = -3; row <= 3; row++) {
      for (let col = -3; col <= 3; col++) {
        const c = { col, row }
        const m = cellCenter(g, c)
        const poly = cellPolygon(g, c)
        let neighbours = 0
        for (let nr = row - 2; nr <= row + 2; nr++) {
          for (let nc = col - 2; nc <= col + 2; nc++) {
            if (nc === col && nr === row) continue
            const n = { col: nc, row: nr }
            if (Math.abs(dist(m, cellCenter(g, n)) - g.sizePx) < 1e-9) {
              neighbours++
              expect(sharedVertices(poly, cellPolygon(g, n))).toBe(2)
            }
          }
        }
        expect(neighbours).toBe(6)
      }
    }
  })

  it('the hex is sizePx wide between parallel sides', () => {
    const poly = cellPolygon(g, { col: 0, row: 0 })
    const xs = poly.map((p) => p.x)
    const ys = poly.map((p) => p.y)
    const w = Math.max(...xs) - Math.min(...xs)
    const h = Math.max(...ys) - Math.min(...ys)
    if (type === 'hexPointy') expect(w).toBeCloseTo(g.sizePx, 9)
    else expect(h).toBeCloseTo(g.sizePx, 9)
  })
})

describe('square grid', () => {
  it('the offset shifts the whole grid', () => {
    const g = gridOf('square')
    expect(cellAt(g, { x: 13.5 + 1, y: -21 + 1 })).toEqual({ col: 0, row: 0 })
    expect(cellAt(g, { x: 13.5 - 1, y: -21 - 1 })).toEqual({ col: -1, row: -1 })
  })
})

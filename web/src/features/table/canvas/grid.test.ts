import { describe, expect, it } from 'vitest'
import type { Grid } from '../../../shared/protocol'
import type { Point } from './camera'
import {
  cellAt,
  cellCenter,
  cellPolygon,
  cellsInRect,
  clipSegment,
  hexEdges,
  intersect,
  cellDistanceFt,
  snapTokenCenter,
  squareLines,
  type Cell,
} from './grid'

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

describe('hexEdges', () => {
  it.each(['hexPointy', 'hexFlat'] as const)(
    '%s: every edge once, all six edges of inner cells',
    (type) => {
      const g = gridOf(type)
      const edges = hexEdges(g, { x: -400, y: -400, w: 800, h: 800 })
      const same = (a: Point, b: Point) => dist(a, b) < 1e-6
      const has = (a: Point, b: Point) =>
        edges.filter(([p, q]) => (same(p, a) && same(q, b)) || (same(p, b) && same(q, a))).length
      for (let i = 0; i < edges.length; i += 7) {
        const e = edges[i]
        if (e === undefined) continue
        expect(has(e[0], e[1])).toBe(1)
      }
      for (const c of [
        { col: 0, row: 0 },
        { col: -1, row: 1 },
        { col: 2, row: -2 },
      ]) {
        const poly = cellPolygon(g, c)
        for (let i = 0; i < 6; i++) {
          const a = poly[i]
          const b = poly[(i + 1) % 6]
          if (a === undefined || b === undefined) throw new Error('missing vertex')
          expect(has(a, b)).toBe(1)
        }
      }
    },
  )
})

describe('squareLines', () => {
  it('lists the lines inside the rect, spanning it', () => {
    const g = gridOf('square')
    const r = { x: 0, y: 0, w: 200, h: 100 }
    const lines = squareLines(g, r)
    // x = 13.5 + 70k in [0, 200]: 83.5 153.5 and 13.5; y = -21 + 70k in [0, 100]: 49
    expect(lines).toHaveLength(3 + 1)
    for (const [a, b] of lines) {
      expect(a.x === b.x || a.y === b.y).toBe(true)
    }
  })
})

describe('clipSegment', () => {
  const r = { x: 0, y: 0, w: 100, h: 50 }

  it('keeps a segment inside', () => {
    expect(clipSegment({ x: 10, y: 10 }, { x: 90, y: 40 }, r)).toEqual([
      { x: 10, y: 10 },
      { x: 90, y: 40 },
    ])
  })

  it('cuts a segment that leaves the rect', () => {
    const got = clipSegment({ x: -50, y: 25 }, { x: 150, y: 25 }, r)
    expect(got).toEqual([
      { x: 0, y: 25 },
      { x: 100, y: 25 },
    ])
  })

  it('drops a segment that misses the rect', () => {
    expect(clipSegment({ x: -10, y: -10 }, { x: -5, y: 100 }, r)).toBeNull()
    expect(clipSegment({ x: 0, y: 60 }, { x: 100, y: 60 }, r)).toBeNull()
  })
})

describe('intersect', () => {
  it('returns the overlap or null', () => {
    expect(intersect({ x: 0, y: 0, w: 10, h: 10 }, { x: 5, y: 5, w: 10, h: 10 })).toEqual({
      x: 5,
      y: 5,
      w: 5,
      h: 5,
    })
    expect(intersect({ x: 0, y: 0, w: 10, h: 10 }, { x: 20, y: 0, w: 5, h: 5 })).toBeNull()
  })
})

describe('snapTokenCenter', () => {
  const g = gridOf('square') // size 70, offset (13.5, -21)
  const at = (col: number, row: number): Point => ({ x: 13.5 + col * 70, y: -21 + row * 70 })

  it('puts odd sizes on a cell centre', () => {
    const near = { x: at(2, 3).x + 20, y: at(2, 3).y + 25 }
    expect(snapTokenCenter(g, near, { w: 1, h: 1 })).toEqual({
      x: at(2, 3).x + 35,
      y: at(2, 3).y + 35,
    })
    expect(snapTokenCenter(g, near, { w: 3, h: 3 })).toEqual({
      x: at(2, 3).x + 35,
      y: at(2, 3).y + 35,
    })
  })

  it('puts even sizes on a cell corner', () => {
    const near = { x: at(2, 3).x + 20, y: at(2, 3).y + 45 }
    expect(snapTokenCenter(g, near, { w: 2, h: 2 })).toEqual(at(2, 4))
  })

  it('snaps each axis by its own size', () => {
    const near = { x: at(2, 3).x + 20, y: at(2, 3).y + 45 }
    expect(snapTokenCenter(g, near, { w: 1, h: 2 })).toEqual({ x: at(2, 3).x + 35, y: at(2, 4).y })
  })

  it.each<Grid['type']>(['hexPointy', 'hexFlat'])('puts a token on a %s cell centre', (type) => {
    const hex = gridOf(type)
    const c = cellCenter(hex, { col: 3, row: 2 })
    const snapped = snapTokenCenter(hex, { x: c.x + 5, y: c.y - 4 }, { w: 2, h: 2 })
    expect(snapped.x).toBeCloseTo(c.x)
    expect(snapped.y).toBeCloseTo(c.y)
  })
})

describe('cellDistanceFt', () => {
  const sq = gridOf('square')
  const centre = (g: Grid, col: number, row: number) => cellCenter(g, { col, row })

  it('counts a square distance by the larger axis', () => {
    expect(cellDistanceFt(sq, centre(sq, 0, 0), centre(sq, 3, 4))).toBe(20)
  })

  it('counts every diagonal as one cell', () => {
    expect(cellDistanceFt(sq, centre(sq, 0, 0), centre(sq, 5, 5))).toBe(25)
  })

  it('uses the unit of the grid', () => {
    const g = { ...sq, unitFt: 10 }
    expect(cellDistanceFt(g, centre(g, 1, 1), centre(g, 4, 2))).toBe(30)
  })

  it.each<Grid['type']>(['hexPointy', 'hexFlat'])('counts %s steps', (type) => {
    const g = gridOf(type)
    // Three steps along the first lattice axis.
    expect(
      cellDistanceFt(
        g,
        centre(g, 2, 2),
        centre(g, type === 'hexFlat' ? 2 : 5, type === 'hexFlat' ? 5 : 2),
      ),
    ).toBe(15)
    expect(cellDistanceFt(g, centre(g, 2, 2), centre(g, 2, 2))).toBe(0)
  })
})

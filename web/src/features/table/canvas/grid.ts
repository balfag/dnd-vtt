import type { Grid } from '../../../shared/protocol'
import type { Point } from './camera'

export type Cell = { col: number; row: number }
export type Rect = { x: number; y: number; w: number; h: number }

const sqrt3 = Math.sqrt(3)

// Hex grids are computed on a "pointy" lattice: odd rows are shifted right by half a cell.
// A flat grid is the same lattice with the axes swapped, so its columns are the lattice rows.

function isOdd(n: number): boolean {
  return Math.abs(n % 2) === 1
}

/** latticeCenter is the centre of a pointy hex of width s, relative to the grid origin. */
function latticeCenter(s: number, col: number, row: number): Point {
  const radius = s / sqrt3
  return {
    x: s / 2 + col * s + (isOdd(row) ? s / 2 : 0),
    y: radius + row * 1.5 * radius,
  }
}

/** latticeCell finds the pointy cell whose centre is nearest to the point (a hex is its Voronoi cell). */
function latticeCell(s: number, p: Point): Cell {
  const rowStep = (s * sqrt3) / 2
  const row0 = Math.floor(p.y / rowStep)
  let best: Cell = { col: 0, row: row0 }
  let bestDist = Infinity
  for (let row = row0 - 1; row <= row0 + 1; row++) {
    const shift = isOdd(row) ? s / 2 : 0
    const col0 = Math.round((p.x - s / 2 - shift) / s)
    for (let col = col0 - 1; col <= col0 + 1; col++) {
      const c = latticeCenter(s, col, row)
      const d = (c.x - p.x) ** 2 + (c.y - p.y) ** 2
      if (d < bestDist) {
        bestDist = d
        best = { col, row }
      }
    }
  }
  return best
}

function localPoint(g: Grid, p: Point): Point {
  const local = { x: p.x - g.offsetX, y: p.y - g.offsetY }
  return g.type === 'hexFlat' ? { x: local.y, y: local.x } : local
}

/** cellAt returns the cell containing the world point. */
export function cellAt(g: Grid, p: Point): Cell {
  if (g.type === 'square') {
    return {
      col: Math.floor((p.x - g.offsetX) / g.sizePx),
      row: Math.floor((p.y - g.offsetY) / g.sizePx),
    }
  }
  const lc = latticeCell(g.sizePx, localPoint(g, p))
  return g.type === 'hexFlat' ? { col: lc.row, row: lc.col } : lc
}

export function cellCenter(g: Grid, c: Cell): Point {
  const s = g.sizePx
  if (g.type === 'square') {
    return { x: g.offsetX + (c.col + 0.5) * s, y: g.offsetY + (c.row + 0.5) * s }
  }
  if (g.type === 'hexFlat') {
    const l = latticeCenter(s, c.row, c.col)
    return { x: g.offsetX + l.y, y: g.offsetY + l.x }
  }
  const l = latticeCenter(s, c.col, c.row)
  return { x: g.offsetX + l.x, y: g.offsetY + l.y }
}

/** cellPolygon returns 4 or 6 vertices, clockwise from the top-left (square, hexFlat) or the top (hexPointy). */
export function cellPolygon(g: Grid, c: Cell): Point[] {
  const s = g.sizePx
  const m = cellCenter(g, c)
  if (g.type === 'square') {
    const h = s / 2
    return [
      { x: m.x - h, y: m.y - h },
      { x: m.x + h, y: m.y - h },
      { x: m.x + h, y: m.y + h },
      { x: m.x - h, y: m.y + h },
    ]
  }
  const r = s / sqrt3
  if (g.type === 'hexPointy') {
    return [
      { x: m.x, y: m.y - r },
      { x: m.x + s / 2, y: m.y - r / 2 },
      { x: m.x + s / 2, y: m.y + r / 2 },
      { x: m.x, y: m.y + r },
      { x: m.x - s / 2, y: m.y + r / 2 },
      { x: m.x - s / 2, y: m.y - r / 2 },
    ]
  }
  return [
    { x: m.x - r / 2, y: m.y - s / 2 },
    { x: m.x + r / 2, y: m.y - s / 2 },
    { x: m.x + r, y: m.y },
    { x: m.x + r / 2, y: m.y + s / 2 },
    { x: m.x - r / 2, y: m.y + s / 2 },
    { x: m.x - r, y: m.y },
  ]
}

function boundsOverlap(poly: Point[], r: Rect): boolean {
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const p of poly) {
    minX = Math.min(minX, p.x)
    minY = Math.min(minY, p.y)
    maxX = Math.max(maxX, p.x)
    maxY = Math.max(maxY, p.y)
  }
  return maxX >= r.x && minX <= r.x + r.w && maxY >= r.y && minY <= r.y + r.h
}

/** cellsInRect lists cells intersecting the world rect (for drawing only what is visible). */
export function cellsInRect(g: Grid, r: Rect): Cell[] {
  const s = g.sizePx
  const cells: Cell[] = []
  if (g.type === 'square') {
    const c0 = Math.floor((r.x - g.offsetX) / s)
    const c1 = Math.floor((r.x + r.w - g.offsetX) / s)
    const r0 = Math.floor((r.y - g.offsetY) / s)
    const r1 = Math.floor((r.y + r.h - g.offsetY) / s)
    for (let row = r0; row <= r1; row++) {
      for (let col = c0; col <= c1; col++) cells.push({ col, row })
    }
    return cells
  }
  // Candidate ranges are padded by one cell, then cut by the polygon bounds.
  const flat = g.type === 'hexFlat'
  const lx0 = (flat ? r.y - g.offsetY : r.x - g.offsetX) / s
  const lx1 = (flat ? r.y + r.h - g.offsetY : r.x + r.w - g.offsetX) / s
  const rowStep = (s * sqrt3) / 2
  const ly0 = (flat ? r.x - g.offsetX : r.y - g.offsetY) / rowStep
  const ly1 = (flat ? r.x + r.w - g.offsetX : r.y + r.h - g.offsetY) / rowStep
  for (let lr = Math.floor(ly0) - 1; lr <= Math.floor(ly1) + 1; lr++) {
    for (let lc = Math.floor(lx0) - 1; lc <= Math.floor(lx1) + 1; lc++) {
      const cell = flat ? { col: lr, row: lc } : { col: lc, row: lr }
      if (boundsOverlap(cellPolygon(g, cell), r)) cells.push(cell)
    }
  }
  return cells
}

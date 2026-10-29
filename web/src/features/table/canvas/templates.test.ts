import { describe, expect, it } from 'vitest'
import type { Grid } from '../../../shared/protocol'
import { affectedCells, templateOutline, type TemplateSpec } from './templates'

const s = 70
const square: Grid = {
  type: 'square',
  sizePx: s,
  unitFt: 5,
  offsetX: 0,
  offsetY: 0,
  color: '#9aa8b3',
  visible: true,
}

const has = (cells: { col: number; row: number }[], col: number, row: number) =>
  cells.some((c) => c.col === col && c.row === row)

describe('affectedCells', () => {
  it('takes 52 cells for a 20 ft sphere centred on a corner', () => {
    const cells = affectedCells({ shape: 'sphere', x: 0, y: 0, size: 20, direction: 0 }, square)
    expect(cells).toHaveLength(52)
  })

  it('takes the same cells for a cylinder', () => {
    const cells = affectedCells({ shape: 'cylinder', x: 0, y: 0, size: 20, direction: 0 }, square)
    expect(cells).toHaveLength(52)
  })

  it('lights the first cell of a cone and not the cell beyond its length', () => {
    // 15 ft right from the middle of the left side of cell (0, 0).
    const cone: TemplateSpec = { shape: 'cone', x: 0, y: s / 2, size: 15, direction: 0 }
    const cells = affectedCells(cone, square)
    expect(has(cells, 0, 0)).toBe(true)
    expect(has(cells, 2, 0)).toBe(true)
    expect(has(cells, 3, 0)).toBe(false)
    // The cone widens: a cell far to the side near the origin is out, the same row far away is in.
    expect(has(cells, 0, 2)).toBe(false)
    expect(has(cells, 2, 1)).toBe(true)
  })

  it('has a half angle of atan(1/2): 45 degrees would take a cell the cone does not reach', () => {
    const cone: TemplateSpec = { shape: 'cone', x: 0, y: s / 2, size: 30, direction: 0 }
    const cells = affectedCells(cone, square)
    // The centre of cell (1, 1) lies 1.5 cells along and 1 cell across: 33.7 degrees off the axis.
    expect(has(cells, 1, 1)).toBe(false)
    // Cell (2, 1) is at 21.8 degrees: inside.
    expect(has(cells, 2, 1)).toBe(true)
  })
  it('takes 6 cells for a 30 ft line', () => {
    const cells = affectedCells({ shape: 'line', x: 0, y: s / 2, size: 30, direction: 0 }, square)
    expect(cells).toHaveLength(6)
    expect(cells.every((c) => c.row === 0)).toBe(true)
  })

  it('takes 9 cells for a 15 ft cube', () => {
    const cells = affectedCells({ shape: 'cube', x: 0, y: 1.5 * s, size: 15, direction: 0 }, square)
    expect(cells).toHaveLength(9)
  })

  it('turns the shape with the direction', () => {
    const down = affectedCells({ shape: 'line', x: s / 2, y: 0, size: 30, direction: 90 }, square)
    expect(down).toHaveLength(6)
    expect(down.every((c) => c.col === 0)).toBe(true)
    const cube = affectedCells({ shape: 'cube', x: 1.5 * s, y: 0, size: 15, direction: 90 }, square)
    expect(cube).toHaveLength(9)
  })

  it('finds cells on hex grids', () => {
    for (const type of ['hexPointy', 'hexFlat'] as const) {
      const g: Grid = { ...square, type }
      const cells = affectedCells({ shape: 'sphere', x: 200, y: 200, size: 15, direction: 0 }, g)
      expect(cells.length).toBeGreaterThan(5)
    }
  })
})

describe('templateOutline', () => {
  it('draws a circle as 64 points at the radius', () => {
    const outline = templateOutline(
      { shape: 'sphere', x: 100, y: 100, size: 10, direction: 0 },
      square,
    )
    expect(outline).toHaveLength(64)
    for (const p of outline) expect(Math.hypot(p.x - 100, p.y - 100)).toBeCloseTo(2 * s)
  })

  it('starts a cone at its origin', () => {
    const outline = templateOutline({ shape: 'cone', x: 5, y: 7, size: 15, direction: 0 }, square)
    expect(outline[0]).toEqual({ x: 5, y: 7 })
  })
})

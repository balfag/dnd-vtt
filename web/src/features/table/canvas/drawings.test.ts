import { describe, expect, it } from 'vitest'
import type { Drawing, Grid } from '../../../shared/protocol'
import { directionDeg, drawingBounds, hitDrawing, rectOfCorners, translateData } from './drawings'

const grid: Grid = {
  type: 'square',
  sizePx: 70,
  unitFt: 5,
  offsetX: 0,
  offsetY: 0,
  color: '#9aa8b3',
  visible: true,
}

const base = { id: 'd1', sceneId: 's1', color: '#e53935', authorPlayerId: 'p1' }
const rect: Drawing = { ...base, kind: 'rect', data: { x: 10, y: 10, w: 100, h: 50 } }
const circle: Drawing = { ...base, kind: 'circle', data: { x: 100, y: 100, r: 40 } }
const line: Drawing = {
  ...base,
  kind: 'freehand',
  data: {
    points: [
      [0, 0],
      [100, 0],
      [100, 100],
    ],
  },
}
const text: Drawing = { ...base, kind: 'text', data: { x: 50, y: 50, text: 'Hello', size: 20 } }
const cube: Drawing = {
  ...base,
  kind: 'template',
  data: { shape: 'cube', x: 0, y: 105, size: 15, direction: 0 },
}

describe('translateData', () => {
  it('moves every kind as a whole', () => {
    expect(translateData(rect, 5, -5)).toEqual({ x: 15, y: 5, w: 100, h: 50 })
    expect(translateData(circle, 5, -5)).toEqual({ x: 105, y: 95, r: 40 })
    expect(translateData(text, 5, -5)).toEqual({ x: 55, y: 45, text: 'Hello', size: 20 })
    expect(translateData(cube, 5, -5)).toEqual({
      shape: 'cube',
      x: 5,
      y: 100,
      size: 15,
      direction: 0,
    })
    expect(translateData(line, 5, -5)).toEqual({
      points: [
        [5, -5],
        [105, -5],
        [105, 95],
      ],
    })
  })
})

describe('hitDrawing', () => {
  it('hits a line near it and misses it far away', () => {
    expect(hitDrawing(line, { x: 50, y: 3 }, 5, grid)).toBe(true)
    expect(hitDrawing(line, { x: 50, y: 30 }, 5, grid)).toBe(false)
  })

  it('hits the outline of a rectangle and a circle, not the inside', () => {
    expect(hitDrawing(rect, { x: 60, y: 11 }, 4, grid)).toBe(true)
    expect(hitDrawing(rect, { x: 60, y: 35 }, 4, grid)).toBe(false)
    expect(hitDrawing(circle, { x: 140, y: 100 }, 4, grid)).toBe(true)
    expect(hitDrawing(circle, { x: 100, y: 100 }, 4, grid)).toBe(false)
  })

  it('hits the box of a text', () => {
    expect(hitDrawing(text, { x: 60, y: 60 }, 2, grid)).toBe(true)
    expect(hitDrawing(text, { x: 300, y: 60 }, 2, grid)).toBe(false)
  })

  it('hits the inside of a template', () => {
    expect(hitDrawing(cube, { x: 100, y: 105 }, 2, grid)).toBe(true)
    expect(hitDrawing(cube, { x: 300, y: 105 }, 2, grid)).toBe(false)
  })
})

describe('drawingBounds', () => {
  it('boxes a line and a circle', () => {
    expect(drawingBounds(line, grid)).toEqual({ x: 0, y: 0, w: 100, h: 100 })
    expect(drawingBounds(circle, grid)).toEqual({ x: 60, y: 60, w: 80, h: 80 })
  })

  it('boxes a template by its outline', () => {
    const b = drawingBounds(cube, grid)
    expect(b.x).toBeCloseTo(0)
    expect(b.w).toBeCloseTo(210)
    expect(b.h).toBeCloseTo(210)
  })
})

describe('rectOfCorners', () => {
  it('puts x, y at the top left whichever corner comes first', () => {
    expect(rectOfCorners({ x: 50, y: 40 }, { x: 10, y: 90 })).toEqual({
      x: 10,
      y: 40,
      w: 40,
      h: 50,
    })
  })
})

describe('directionDeg', () => {
  it('counts degrees clockwise from the X axis into [0, 360)', () => {
    expect(directionDeg({ x: 0, y: 0 }, { x: 10, y: 0 })).toBe(0)
    expect(directionDeg({ x: 0, y: 0 }, { x: 0, y: 10 })).toBe(90)
    expect(directionDeg({ x: 0, y: 0 }, { x: -10, y: 0 })).toBe(180)
    expect(directionDeg({ x: 0, y: 0 }, { x: 0, y: -10 })).toBe(270)
  })
})

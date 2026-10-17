import { describe, expect, it } from 'vitest'
import type { Shape } from '../../../shared/protocol'
import {
  brushStamps,
  clampToScene,
  MAX_BRUSH_STAMPS,
  polygonFromPoints,
  rectFromPoints,
} from './fogShapes'

const scene = { width: 1000, height: 500 }

function circles(shapes: Shape[]): { x: number; y: number; r: number }[] {
  return shapes.map((s) => {
    if (s.type !== 'circle') throw new Error('not a circle')
    return s
  })
}

describe('rectFromPoints', () => {
  it('is the same rectangle whatever the corner order', () => {
    const want: Shape = { type: 'rect', x: 10, y: 20, w: 30, h: 40 }
    expect(rectFromPoints({ x: 10, y: 20 }, { x: 40, y: 60 })).toStrictEqual(want)
    expect(rectFromPoints({ x: 40, y: 60 }, { x: 10, y: 20 })).toStrictEqual(want)
    expect(rectFromPoints({ x: 40, y: 20 }, { x: 10, y: 60 })).toStrictEqual(want)
    expect(rectFromPoints({ x: 10, y: 60 }, { x: 40, y: 20 })).toStrictEqual(want)
  })
})

describe('polygonFromPoints', () => {
  const pt = (x: number, y: number) => ({ x, y })

  it('makes a polygon of 3 points', () => {
    expect(polygonFromPoints([pt(0, 0), pt(10, 0), pt(10, 10)])).toStrictEqual({
      type: 'polygon',
      points: [
        [0, 0],
        [10, 0],
        [10, 10],
      ],
    })
  })

  it('is null for 2 points', () => {
    expect(polygonFromPoints([pt(0, 0), pt(10, 0)])).toBeNull()
  })

  it('is null for 201 points and fine for 200', () => {
    const many = (n: number) => Array.from({ length: n }, (_, i) => pt(i, i % 7))
    expect(polygonFromPoints(many(201))).toBeNull()
    expect(polygonFromPoints(many(200))).not.toBeNull()
  })

  it('drops consecutive duplicates', () => {
    const shape = polygonFromPoints([pt(0, 0), pt(0, 0), pt(10, 0), pt(10, 0), pt(10, 10)])
    expect(shape).toStrictEqual({
      type: 'polygon',
      points: [
        [0, 0],
        [10, 0],
        [10, 10],
      ],
    })
  })

  it('counts points after dropping duplicates', () => {
    expect(polygonFromPoints([pt(0, 0), pt(0, 0), pt(10, 0), pt(10, 0)])).toBeNull()
  })
})

describe('brushStamps', () => {
  it('is one circle for a click', () => {
    expect(brushStamps([{ x: 5, y: 6 }], 10)).toStrictEqual([{ type: 'circle', x: 5, y: 6, r: 10 }])
  })

  it('is one circle for a stroke shorter than the radius', () => {
    const stamps = brushStamps(
      [
        { x: 0, y: 0 },
        { x: 6, y: 0 },
      ],
      10,
    )
    expect(stamps).toStrictEqual([{ type: 'circle', x: 0, y: 0, r: 10 }])
  })

  it('puts circles r apart along the stroke', () => {
    const stamps = circles(
      brushStamps(
        [
          { x: 0, y: 0 },
          { x: 30, y: 0 },
          { x: 30, y: 20 },
        ],
        10,
      ),
    )
    expect(stamps.map((s) => [s.x, s.y])).toStrictEqual([
      [0, 0],
      [10, 0],
      [20, 0],
      [30, 0],
      [30, 10],
      [30, 20],
    ])
  })

  it('thins a long stroke to 20 circles, evenly, from the start to the end', () => {
    const stamps = circles(
      brushStamps(
        [
          { x: 0, y: 0 },
          { x: 1900, y: 0 },
        ],
        10,
      ),
    )
    expect(stamps).toHaveLength(MAX_BRUSH_STAMPS)
    expect(stamps[0]?.x).toBe(0)
    expect(stamps[19]?.x).toBeCloseTo(1900)
    for (let i = 1; i < stamps.length; i++) {
      expect((stamps[i]?.x ?? 0) - (stamps[i - 1]?.x ?? 0)).toBeCloseTo(100)
    }
  })

  it('is empty without a stroke or a radius', () => {
    expect(brushStamps([], 10)).toStrictEqual([])
    expect(brushStamps([{ x: 0, y: 0 }], 0)).toStrictEqual([])
  })
})

describe('clampToScene', () => {
  it('cuts a rectangle at the scene edge', () => {
    expect(clampToScene({ type: 'rect', x: -50, y: 450, w: 100, h: 200 }, scene)).toStrictEqual({
      type: 'rect',
      x: 0,
      y: 450,
      w: 50,
      h: 50,
    })
  })

  it('leaves a rectangle inside the scene as it is', () => {
    const s: Shape = { type: 'rect', x: 1, y: 2, w: 3, h: 4 }
    expect(clampToScene(s, scene)).toStrictEqual(s)
  })

  it('keeps the centre of a circle inside', () => {
    expect(clampToScene({ type: 'circle', x: 1200, y: -5, r: 30 }, scene)).toStrictEqual({
      type: 'circle',
      x: 1000,
      y: 0,
      r: 30,
    })
  })

  it('moves the points of a polygon into the scene', () => {
    expect(
      clampToScene(
        {
          type: 'polygon',
          points: [
            [-10, 20],
            [1100, 600],
            [500, 250],
          ],
        },
        scene,
      ),
    ).toStrictEqual({
      type: 'polygon',
      points: [
        [0, 20],
        [1000, 500],
        [500, 250],
      ],
    })
  })
})

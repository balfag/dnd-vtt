import type { Shape } from '../../../shared/protocol'
import type { Point } from './camera'

/** The same limits as on the server (protocol.md, «Туман»). */
export const MIN_POLYGON_POINTS = 3
export const MAX_POLYGON_POINTS = 200
/** One brush stroke is at most this many fog operations. */
export const MAX_BRUSH_STAMPS = 20

/** FogShapeKind is what a fog tool draws: a rectangle, a polygon, or a brush stroke (a row of circles). */
export type FogShapeKind = 'rect' | 'polygon' | 'brush'

/** rectFromPoints is the rectangle with the corners a and b, in any order. */
export function rectFromPoints(a: Point, b: Point): Shape {
  return {
    type: 'rect',
    x: Math.min(a.x, b.x),
    y: Math.min(a.y, b.y),
    w: Math.abs(a.x - b.x),
    h: Math.abs(a.y - b.y),
  }
}

/** polygonFromPoints is the polygon through points; null if it has fewer than 3 or more than 200 points. */
export function polygonFromPoints(points: Point[]): Shape | null {
  const corners: [number, number][] = []
  for (const p of points) {
    const last = corners[corners.length - 1]
    if (last !== undefined && last[0] === p.x && last[1] === p.y) continue
    corners.push([p.x, p.y])
  }
  if (corners.length < MIN_POLYGON_POINTS || corners.length > MAX_POLYGON_POINTS) return null
  return { type: 'polygon', points: corners }
}

function pointAt(stroke: Point[], lengths: number[], distance: number): Point {
  let i = 1
  while (i < lengths.length - 1 && (lengths[i] ?? 0) < distance) i++
  const a = stroke[i - 1]
  const b = stroke[i]
  const from = lengths[i - 1]
  const to = lengths[i]
  if (a === undefined || b === undefined || from === undefined || to === undefined || to <= from) {
    return a ?? { x: 0, y: 0 }
  }
  const k = Math.min(1, Math.max(0, (distance - from) / (to - from)))
  return { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k }
}

/** brushStamps samples a stroke into circles of radius r spaced r apart, at most 20 per stroke. */
export function brushStamps(stroke: Point[], r: number): Shape[] {
  const first = stroke[0]
  if (first === undefined || !(r > 0)) return []
  const lengths = [0]
  for (let i = 1; i < stroke.length; i++) {
    const a = stroke[i - 1]
    const b = stroke[i]
    if (a === undefined || b === undefined) continue
    lengths.push((lengths[i - 1] ?? 0) + Math.hypot(b.x - a.x, b.y - a.y))
  }
  const total = lengths[lengths.length - 1] ?? 0
  const wanted = Math.floor(total / r) + 1
  if (wanted <= 1) return [{ type: 'circle', x: first.x, y: first.y, r }]
  const count = Math.min(wanted, MAX_BRUSH_STAMPS)
  // A long stroke is thinned evenly: the first and the last stamp stay where the stroke starts and ends.
  const spacing = wanted > MAX_BRUSH_STAMPS ? total / (count - 1) : r
  const stamps: Shape[] = []
  for (let i = 0; i < count; i++) {
    const at = pointAt(stroke, lengths, i * spacing)
    stamps.push({ type: 'circle', x: at.x, y: at.y, r })
  }
  return stamps
}

function clamp(v: number, max: number): number {
  return Math.min(max, Math.max(0, v))
}

/** clampToScene keeps the shape's coordinates inside the scene (a circle keeps its centre inside). */
export function clampToScene(s: Shape, scene: { width: number; height: number }): Shape {
  switch (s.type) {
    case 'rect': {
      const x0 = clamp(s.x, scene.width)
      const y0 = clamp(s.y, scene.height)
      const x1 = clamp(s.x + s.w, scene.width)
      const y1 = clamp(s.y + s.h, scene.height)
      return { type: 'rect', x: x0, y: y0, w: x1 - x0, h: y1 - y0 }
    }
    case 'circle':
      return { type: 'circle', x: clamp(s.x, scene.width), y: clamp(s.y, scene.height), r: s.r }
    case 'polygon':
      return {
        type: 'polygon',
        points: s.points.map(([x, y]): [number, number] => [
          clamp(x, scene.width),
          clamp(y, scene.height),
        ]),
      }
  }
}

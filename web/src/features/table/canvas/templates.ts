import type { Grid, TemplateShape } from '../../../shared/protocol'
import type { Point } from './camera'
import { cellCenter, cellsInRect, type Cell } from './grid'

export type { TemplateShape }

/** TemplateSpec is where a template stands: the origin or centre, the size in feet, the direction in degrees. */
export interface TemplateSpec {
  shape: TemplateShape
  x: number
  y: number
  size: number
  direction: number
}

// The width of a cone at any distance is that distance: half the angle is atan(1/2).
const coneHalfAngle = Math.atan(0.5)
const coneArcSteps = 24
const circleSteps = 64
const lineWidthFt = 5
// A point this close to an edge is on it (rotated shapes carry rounding noise).
const edgeTolerance = 1e-6

function isCircle(shape: TemplateShape): boolean {
  return shape === 'sphere' || shape === 'cylinder'
}

/** rectAlong is a rectangle from the origin along the direction, centred on its axis. */
function rectAlong(o: Point, angle: number, length: number, width: number): Point[] {
  const dx = Math.cos(angle)
  const dy = Math.sin(angle)
  const nx = -dy * (width / 2)
  const ny = dx * (width / 2)
  const ex = o.x + dx * length
  const ey = o.y + dy * length
  return [
    { x: o.x + nx, y: o.y + ny },
    { x: ex + nx, y: ey + ny },
    { x: ex - nx, y: ey - ny },
    { x: o.x - nx, y: o.y - ny },
  ]
}

/** templateOutline returns the template polygon in world units (circles as 64-gon). */
export function templateOutline(t: TemplateSpec, g: Grid): Point[] {
  const length = (t.size / g.unitFt) * g.sizePx
  const o = { x: t.x, y: t.y }
  const angle = (t.direction * Math.PI) / 180
  switch (t.shape) {
    case 'sphere':
    case 'cylinder': {
      const points: Point[] = []
      for (let i = 0; i < circleSteps; i++) {
        const a = (i * 2 * Math.PI) / circleSteps
        points.push({ x: o.x + length * Math.cos(a), y: o.y + length * Math.sin(a) })
      }
      return points
    }
    case 'cone': {
      const points: Point[] = [o]
      for (let i = 0; i <= coneArcSteps; i++) {
        const a = angle - coneHalfAngle + (i * 2 * coneHalfAngle) / coneArcSteps
        points.push({ x: o.x + length * Math.cos(a), y: o.y + length * Math.sin(a) })
      }
      return points
    }
    case 'line':
      return rectAlong(o, angle, length, (lineWidthFt / g.unitFt) * g.sizePx)
    case 'cube':
      return rectAlong(o, angle, length, length)
  }
}

function distanceToSegment(p: Point, a: Point, b: Point): number {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len2 = dx * dx + dy * dy
  const s = len2 === 0 ? 0 : Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2))
  return Math.hypot(p.x - (a.x + s * dx), p.y - (a.y + s * dy))
}

/** insideOrOn: even-odd containment, the boundary counts. */
function insideOrOn(p: Point, poly: Point[]): boolean {
  let inside = false
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[i]
    const b = poly[j]
    if (a === undefined || b === undefined) continue
    if (distanceToSegment(p, a, b) <= edgeTolerance) return true
    if (a.y > p.y !== b.y > p.y && p.x < ((b.x - a.x) * (p.y - a.y)) / (b.y - a.y) + a.x) {
      inside = !inside
    }
  }
  return inside
}

/** affectedCells: a cell is affected when its centre lies inside the outline (boundary counts). */
export function affectedCells(t: TemplateSpec, g: Grid): Cell[] {
  const outline = templateOutline(t, g)
  const xs = outline.map((p) => p.x)
  const ys = outline.map((p) => p.y)
  const minX = Math.min(...xs)
  const minY = Math.min(...ys)
  const bounds = { x: minX, y: minY, w: Math.max(...xs) - minX, h: Math.max(...ys) - minY }
  const radius = (t.size / g.unitFt) * g.sizePx
  return cellsInRect(g, bounds).filter((c) => {
    const m = cellCenter(g, c)
    // The 64-gon is only for drawing: a sphere and a cylinder are tested against the true circle.
    if (isCircle(t.shape)) return Math.hypot(m.x - t.x, m.y - t.y) <= radius + edgeTolerance
    return insideOrOn(m, outline)
  })
}

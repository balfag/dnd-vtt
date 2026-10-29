import type { Drawing, DrawingContent, Grid } from '../../../shared/protocol'
import type { Point } from './camera'
import type { Rect } from './grid'
import { distanceToSegment } from './simplify'
import { insideOrOn, templateOutline } from './templates'

/** The width of a character of a text drawing, in sizes (a guess: the text is measured by the renderer). */
const textCharWidth = 0.5
const textLineHeight = 1.2

/** translateData moves a drawing as a whole by (dx, dy) world units. */
export function translateData(d: DrawingContent, dx: number, dy: number): DrawingContent['data'] {
  switch (d.kind) {
    case 'freehand':
      return { points: d.data.points.map(([x, y]): [number, number] => [x + dx, y + dy]) }
    case 'rect':
    case 'circle':
    case 'text':
    case 'template':
      return { ...d.data, x: d.data.x + dx, y: d.data.y + dy }
  }
}

/** drawingBounds is the box around a drawing in world units. */
export function drawingBounds(d: Drawing, g: Grid): Rect {
  switch (d.kind) {
    case 'freehand': {
      const xs = d.data.points.map(([x]) => x)
      const ys = d.data.points.map(([, y]) => y)
      const x = Math.min(...xs)
      const y = Math.min(...ys)
      return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y }
    }
    case 'rect':
      return { x: d.data.x, y: d.data.y, w: d.data.w, h: d.data.h }
    case 'circle':
      return { x: d.data.x - d.data.r, y: d.data.y - d.data.r, w: d.data.r * 2, h: d.data.r * 2 }
    case 'text':
      return {
        x: d.data.x,
        y: d.data.y,
        w: d.data.text.length * d.data.size * textCharWidth,
        h: d.data.size * textLineHeight,
      }
    case 'template': {
      const outline = templateOutline(d.data, g)
      const xs = outline.map((p) => p.x)
      const ys = outline.map((p) => p.y)
      const x = Math.min(...xs)
      const y = Math.min(...ys)
      return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y }
    }
  }
}

/**
 * hitDrawing tells whether the world point is on the drawing: within tol of a line, inside a
 * template or a text. A rectangle and a circle are outlines: their inside is empty.
 */
export function hitDrawing(d: Drawing, p: Point, tol: number, g: Grid): boolean {
  switch (d.kind) {
    case 'freehand': {
      const pts = d.data.points
      for (let i = 1; i < pts.length; i++) {
        const a = pts[i - 1]
        const b = pts[i]
        if (
          a !== undefined &&
          b !== undefined &&
          distanceToSegment(p, { x: a[0], y: a[1] }, { x: b[0], y: b[1] }) <= tol
        ) {
          return true
        }
      }
      return false
    }
    case 'rect': {
      const { x, y, w, h } = d.data
      return (
        distanceToSegment(p, { x: x, y: y }, { x: x + w, y: y }) <= tol ||
        distanceToSegment(p, { x: x + w, y: y }, { x: x + w, y: y + h }) <= tol ||
        distanceToSegment(p, { x: x + w, y: y + h }, { x: x, y: y + h }) <= tol ||
        distanceToSegment(p, { x: x, y: y + h }, { x: x, y: y }) <= tol
      )
    }
    case 'circle':
      return Math.abs(Math.hypot(p.x - d.data.x, p.y - d.data.y) - d.data.r) <= tol
    case 'text': {
      const b = drawingBounds(d, g)
      return (
        p.x >= b.x - tol && p.x <= b.x + b.w + tol && p.y >= b.y - tol && p.y <= b.y + b.h + tol
      )
    }
    case 'template':
      return insideOrOn(p, templateOutline(d.data, g))
  }
}

/** rectOfCorners is the rectangle between two corners, with x, y at the top left. */
export function rectOfCorners(a: Point, b: Point): { x: number; y: number; w: number; h: number } {
  return {
    x: Math.min(a.x, b.x),
    y: Math.min(a.y, b.y),
    w: Math.abs(a.x - b.x),
    h: Math.abs(a.y - b.y),
  }
}

/** directionDeg is the direction from a to b in degrees clockwise from the X axis, in [0, 360). */
export function directionDeg(a: Point, b: Point): number {
  const deg = (Math.atan2(b.y - a.y, b.x - a.x) * 180) / Math.PI
  return deg < 0 ? deg + 360 : deg
}

import type { Drawing, DrawingContent } from '../../shared/protocol'
import type { Point } from '../table/canvas/camera'
import { rectOfCorners } from '../table/canvas/drawings'
import { simplify, thin } from '../table/canvas/simplify'
import {
  createDrawing,
  TEXT_LENGTH_MAX,
  TEXT_SIZE_MAX,
  TEXT_SIZE_MIN,
  type ToolEnv,
  type ToolPointer,
} from './toolState'

/** A line keeps at most this many points (the server takes 2000). */
export const MAX_FREEHAND_POINTS = 2000
// Strokes are simplified to this many screen px.
const simplifyPx = 1.5
// A rectangle or a circle smaller than this on the screen is a stray click, not a shape.
const minShapePx = 3
const previewId = 'preview'

/** textLength tells whether the text may be a drawing: 1–500 characters. */
export function validText(text: string): boolean {
  return text.length >= 1 && text.length <= TEXT_LENGTH_MAX
}

/** clampTextSize keeps the size of a text within what the server takes. */
export function clampTextSize(size: number): number {
  return Math.min(TEXT_SIZE_MAX, Math.max(TEXT_SIZE_MIN, Math.round(size)))
}

/** freehandPoints makes the points of a line to send: simplified for the zoom, thinned to the limit; null for a speck. */
export function freehandPoints(points: Point[], zoom: number): [number, number][] | null {
  const kept = thin(simplify(points, simplifyPx / zoom), MAX_FREEHAND_POINTS)
  if (kept.length < 2) return null
  return kept.map((p): [number, number] => [p.x, p.y])
}

type Stroke =
  { kind: 'freehand'; points: Point[] } | { kind: 'rect' | 'circle'; from: Point; to: Point }

/** DrawTool draws a line, a rectangle or a circle by dragging; a text is typed where the user clicked. */
export class DrawTool {
  private stroke: Stroke | null = null
  private pointerId: number | null = null

  private readonly env: ToolEnv

  constructor(env: ToolEnv) {
    this.env = env
  }

  /** color is the colour the next drawing gets. */
  color(): string {
    return this.env.drawOptions().color ?? this.env.myColor()
  }

  /** drawing tells that a stroke is under the pointer. */
  drawing(): boolean {
    return this.stroke !== null
  }

  /** down starts a stroke; for a text it returns true: the canvas asks for the words. */
  down(p: ToolPointer, pointerId: number): 'stroke' | 'text' {
    const kind = this.env.drawOptions().kind
    if (kind === 'text') return 'text'
    this.pointerId = pointerId
    this.stroke =
      kind === 'freehand' ? { kind, points: [p.world] } : { kind, from: p.world, to: p.world }
    this.env.changed()
    return 'stroke'
  }

  move(p: ToolPointer, pointerId: number): void {
    const s = this.stroke
    if (s === null || pointerId !== this.pointerId) return
    if (s.kind === 'freehand') {
      const last = s.points[s.points.length - 1]
      // A point closer than a screen pixel adds nothing.
      if (
        last !== undefined &&
        Math.hypot(p.world.x - last.x, p.world.y - last.y) * this.env.zoom() < 1
      ) {
        return
      }
      s.points.push(p.world)
    } else {
      s.to = p.world
    }
    this.env.changed()
  }

  up(p: ToolPointer, pointerId: number): void {
    const s = this.stroke
    if (s === null || pointerId !== this.pointerId) return
    this.move(p, pointerId)
    this.stroke = null
    this.pointerId = null
    const content = this.content(s)
    if (content !== null) createDrawing(this.env, content, this.color())
    this.env.changed()
  }

  /** cancel drops the stroke. */
  cancel(): void {
    if (this.stroke === null) return
    this.stroke = null
    this.pointerId = null
    this.env.changed()
  }

  /** createText puts a text at a point; false when the text cannot be a drawing. */
  createText(at: Point, text: string): boolean {
    if (!validText(text)) return false
    createDrawing(
      this.env,
      {
        kind: 'text',
        data: { x: at.x, y: at.y, text, size: clampTextSize(this.env.drawOptions().textSize) },
      },
      this.color(),
    )
    return true
  }

  /** preview is the stroke as a drawing for the overlay, or null. */
  preview(authorPlayerId: string): Drawing | null {
    const scene = this.env.scene()
    const s = this.stroke
    if (scene === null || s === null) return null
    const content = this.content(s, true)
    if (content === null) return null
    return { ...content, id: previewId, sceneId: scene.id, color: this.color(), authorPlayerId }
  }

  private content(s: Stroke, forPreview = false): DrawingContent | null {
    const zoom = this.env.zoom()
    if (s.kind === 'freehand') {
      // The preview shows the raw line; what is sent is simplified.
      const points = forPreview
        ? s.points.map((p): [number, number] => [p.x, p.y])
        : freehandPoints(s.points, zoom)
      return points === null || points.length < 2 ? null : { kind: 'freehand', data: { points } }
    }
    if (s.kind === 'rect') {
      const r = rectOfCorners(s.from, s.to)
      if (r.w * zoom < minShapePx || r.h * zoom < minShapePx) return null
      return { kind: 'rect', data: r }
    }
    const radius = Math.hypot(s.to.x - s.from.x, s.to.y - s.from.y)
    if (radius * zoom < minShapePx) return null
    return { kind: 'circle', data: { x: s.from.x, y: s.from.y, r: radius } }
  }
}

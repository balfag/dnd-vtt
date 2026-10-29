import type { Drawing } from '../../shared/protocol'
import type { Point } from '../table/canvas/camera'
import { directionDeg } from '../table/canvas/drawings'
import {
  createDrawing,
  TEMPLATE_COLOR,
  type TemplateOptions,
  type ToolEnv,
  type ToolPointer,
} from './toolState'

// A drag shorter than this (screen px) does not turn the template: it points along the X axis.
const turnSlopPx = 6
const previewId = 'preview'

/** isRound tells shapes whose direction does not matter. */
function isRound(shape: TemplateOptions['shape']): boolean {
  return shape === 'sphere' || shape === 'cylinder'
}

/** TemplateTool puts an area template: press at the origin and drag to turn it, release to place it. */
export class TemplateTool {
  private origin: Point | null = null
  private direction = 0
  private pointerId: number | null = null

  private readonly env: ToolEnv

  constructor(env: ToolEnv) {
    this.env = env
  }

  down(p: ToolPointer, pointerId: number): void {
    this.origin = p.world
    this.direction = 0
    this.pointerId = pointerId
    this.env.changed()
  }

  /** move turns the template toward the pointer. */
  move(p: ToolPointer, pointerId: number): void {
    const o = this.origin
    if (o === null || pointerId !== this.pointerId) return
    const far = Math.hypot(p.world.x - o.x, p.world.y - o.y) * this.env.zoom() > turnSlopPx
    if (far && !isRound(this.env.templateOptions().shape)) this.direction = directionDeg(o, p.world)
    this.env.changed()
  }

  up(p: ToolPointer, pointerId: number): void {
    const o = this.origin
    if (o === null || pointerId !== this.pointerId) return
    this.move(p, pointerId)
    const { shape, size } = this.env.templateOptions()
    const direction = isRound(shape) ? 0 : this.direction
    this.origin = null
    this.pointerId = null
    createDrawing(
      this.env,
      { kind: 'template', data: { shape, x: o.x, y: o.y, size, direction } },
      TEMPLATE_COLOR,
    )
    this.env.changed()
  }

  cancel(): void {
    if (this.origin === null) return
    this.origin = null
    this.pointerId = null
    this.env.changed()
  }

  /** preview is the template being placed as a drawing for the overlay, or null. */
  preview(authorPlayerId: string): Drawing | null {
    const scene = this.env.scene()
    const o = this.origin
    if (scene === null || o === null) return null
    const { shape, size } = this.env.templateOptions()
    return {
      kind: 'template',
      data: { shape, x: o.x, y: o.y, size, direction: isRound(shape) ? 0 : this.direction },
      id: previewId,
      sceneId: scene.id,
      color: TEMPLATE_COLOR,
      authorPlayerId,
    }
  }
}

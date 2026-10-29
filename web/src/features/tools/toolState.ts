import type { DrawingContent, SceneView, TemplateShape } from '../../shared/protocol'
import { MessageType } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import type { Point } from '../table/canvas/camera'

/** DrawKind is what the draw tool puts on the map. */
export type DrawKind = 'freehand' | 'rect' | 'circle' | 'text'

/** DrawOptions is how the draw tool draws; color is #rrggbb. */
export interface DrawOptions {
  kind: DrawKind
  /** null: the colour of the player who draws. */
  color: string | null
  /** The size of a text, px of the world. */
  textSize: number
}

/** TemplateOptions is how the template tool draws. */
export interface TemplateOptions {
  shape: TemplateShape
  /** Feet: the length of a cone and a line, the radius of a sphere and a cylinder, the edge of a cube. */
  size: number
}

/** The colours of the players' palette (internal/game); the draw tool offers them. */
export const DRAW_COLORS = [
  '#e53935',
  '#1e88e5',
  '#43a047',
  '#fb8c00',
  '#8e24aa',
  '#00acc1',
  '#fdd835',
  '#6d4c41',
] as const

/** TEMPLATE_COLOR is --verdigris: the colour of every area template. */
export const TEMPLATE_COLOR = '#4fa58e'

/** gmTools are the tools only the GM has. */
export const GM_TOOLS: readonly string[] = ['place', 'reveal', 'hide']

export const TEXT_SIZE_MIN = 8
export const TEXT_SIZE_MAX = 200
export const TEXT_LENGTH_MAX = 500
export const TEMPLATE_SIZE_MIN = 5
export const TEMPLATE_SIZE_MAX = 1000

export const TEMPLATE_SHAPES: readonly TemplateShape[] = [
  'cone',
  'sphere',
  'line',
  'cube',
  'cylinder',
]

export const DEFAULT_DRAW: DrawOptions = { kind: 'freehand', color: null, textSize: 24 }

const defaultTemplateSizes: Record<TemplateShape, number> = {
  cone: 15,
  sphere: 20,
  line: 30,
  cube: 15,
  cylinder: 10,
}

/** defaultTemplateSize is the size in feet a shape starts with. */
export function defaultTemplateSize(shape: TemplateShape): number {
  return defaultTemplateSizes[shape]
}

export const DEFAULT_TEMPLATE: TemplateOptions = { shape: 'cone', size: defaultTemplateSizes.cone }

/** ToolPointer is a press or a move on the canvas: the same point in world units and in the host's pixels. */
export interface ToolPointer {
  world: Point
  screen: Point
  shiftKey: boolean
}

/** ToolEnv is what a tool needs from the canvas and the connection. */
export interface ToolEnv {
  scene(): SceneView | null
  zoom(): number
  drawOptions(): DrawOptions
  templateOptions(): TemplateOptions
  /** myColor is the colour of the viewer: the draw tool's default. */
  myColor(): string
  /** transient sends a message that has no answer and is never replayed. */
  transient(type: string, data: unknown): void
  command(type: string, data: unknown): Promise<unknown>
  announce(text: string, visible: boolean): void
  t(key: string, options?: Record<string, unknown>): string
  /** changed tells that the state shown on the canvas is different now. */
  changed(): void
}

/** createDrawing asks the server for a drawing; a refusal is announced. */
export function createDrawing(env: ToolEnv, content: DrawingContent, color: string): void {
  const scene = env.scene()
  if (scene === null) return
  env
    .command(MessageType.DrawingCreate, {
      sceneId: scene.id,
      kind: content.kind,
      data: content.data,
      color,
    })
    .catch((err: unknown) => {
      env.announce(env.t(commandErrorKey(err)), true)
    })
}

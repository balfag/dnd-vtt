import { MessageType } from '../../shared/protocol'
import type { Point } from '../table/canvas/camera'
import { rulerDistanceFt, rulerPath } from '../table/canvas/ruler'
import type { ToolEnv, ToolPointer } from './toolState'

/** At most 20 updates a second, so the server has little to drop. */
export const RULER_SEND_EVERY_MS = 50
/** The server takes 1–50 points. */
const maxPoints = 50
// A press that moves less than this (screen px) is a click.
const clickSlopPx = 4

type Mode =
  | 'none'
  /** The button is down: the end follows the pointer. */
  | 'pressed'
  /** A click set the start or a waypoint: the end follows the pointer without a button. */
  | 'following'
  /** The drag is over: the ruler stays where it is until it is cleared. */
  | 'fixed'

/**
 * RulerTool measures a path over the cells. A drag draws a segment; a click without a drag adds
 * a waypoint and the next move pulls a new segment from it; a double click, Escape or the right
 * button remove the ruler. While it exists, it is sent for the others to see.
 */
export class RulerTool {
  private mode: Mode = 'none'
  private waypoints: Point[] = []
  private cursor: Point | null = null
  private pressedAt: Point | null = null
  private pointerId: number | null = null
  private sentKey = ''
  private sentScene: string | null = null
  private lastSent = Number.NEGATIVE_INFINITY
  private timer: ReturnType<typeof setTimeout> | null = null

  private readonly env: ToolEnv
  private readonly now: () => number

  constructor(env: ToolEnv, now: () => number = Date.now) {
    this.env = env
    this.now = now
  }

  /** has tells that there is a ruler to show or to remove. */
  has(): boolean {
    return this.mode !== 'none'
  }

  /** view is the ruler as it is now: the path through cell centres and its length. */
  view(): { path: Point[]; ft: number } | null {
    const scene = this.env.scene()
    if (scene === null || this.mode === 'none' || this.waypoints.length === 0) return null
    const raw = this.cursor === null ? this.waypoints : [...this.waypoints, this.cursor]
    const path = rulerPath(scene.grid, raw)
    return { path, ft: rulerDistanceFt(scene.grid, path) }
  }

  down(p: ToolPointer, pointerId: number): void {
    if (this.mode === 'following') {
      // The click is a waypoint; the segment to it is already on the screen.
      if (this.waypoints.length < maxPoints - 1) this.waypoints = [...this.waypoints, p.world]
      this.cursor = p.world
      this.mode = 'pressed'
    } else {
      this.waypoints = [p.world]
      this.cursor = p.world
      this.mode = 'pressed'
    }
    this.pressedAt = p.screen
    this.pointerId = pointerId
    this.update()
  }

  move(p: ToolPointer): void {
    if (this.mode !== 'pressed' && this.mode !== 'following') return
    this.cursor = p.world
    this.update()
  }

  up(p: ToolPointer, pointerId: number): void {
    if (this.mode !== 'pressed' || pointerId !== this.pointerId) return
    const from = this.pressedAt
    this.pointerId = null
    this.pressedAt = null
    const moved =
      from !== null && Math.hypot(p.screen.x - from.x, p.screen.y - from.y) > clickSlopPx
    this.cursor = p.world
    // A drag leaves the ruler as it is; a click lets the next move pull a segment.
    this.mode = moved ? 'fixed' : 'following'
    this.update()
  }

  /** clear removes the ruler and tells the others. */
  clear(): void {
    if (this.timer !== null) clearTimeout(this.timer)
    this.timer = null
    const hadSent = this.sentScene !== null
    const scene = this.sentScene
    this.mode = 'none'
    this.waypoints = []
    this.cursor = null
    this.pressedAt = null
    this.pointerId = null
    this.sentKey = ''
    this.sentScene = null
    if (hadSent && scene !== null) this.env.transient(MessageType.RulerClear, { sceneId: scene })
    this.env.changed()
  }

  private update(): void {
    this.env.changed()
    this.schedule()
  }

  /** schedule sends the ruler now, or at the end of the pause the rate limit asks for. */
  private schedule(): void {
    if (this.timer !== null) return
    const wait = this.lastSent + RULER_SEND_EVERY_MS - this.now()
    if (wait <= 0) {
      this.send()
      return
    }
    this.timer = setTimeout(() => {
      this.timer = null
      this.send()
    }, wait)
  }

  private send(): void {
    const scene = this.env.scene()
    const v = this.view()
    if (scene === null || v === null) return
    const points = v.path.slice(0, maxPoints).map((q): [number, number] => [q.x, q.y])
    const key = JSON.stringify(points)
    if (key === this.sentKey && scene.id === this.sentScene) return
    this.sentKey = key
    this.sentScene = scene.id
    this.lastSent = this.now()
    this.env.transient(MessageType.RulerUpdate, { sceneId: scene.id, points })
  }
}

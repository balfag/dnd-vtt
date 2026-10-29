import { MessageType } from '../../shared/protocol'
import type { Point } from '../table/canvas/camera'
import type { ToolEnv } from './toolState'

/**
 * placePing shows a ping at a point of the scene to everyone. The GM's ping with Shift also
 * moves the cameras of the players; for a player Shift does nothing.
 */
export function placePing(
  env: ToolEnv,
  at: Point,
  opts: { isGM: boolean; shiftKey: boolean },
): void {
  const scene = env.scene()
  if (scene === null) return
  const focus = opts.isGM && opts.shiftKey
  env.transient(MessageType.PingPlace, {
    sceneId: scene.id,
    x: at.x,
    y: at.y,
    ...(focus ? { focus: true } : {}),
  })
}

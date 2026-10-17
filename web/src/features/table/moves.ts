import type { Token } from '../../shared/protocol'
import type { Point } from './canvas/camera'

/** Pending moves the client shows before the server confirms them. */
export function createMoves(): {
  start(tokenId: string, to: Point): void
  confirm(tokenId: string): void // token.changed / token.removed for it arrived
  fail(tokenId: string): void // command error: show the server position again
  position(t: Token): Point // pending ?? server
} {
  const pending = new Map<string, Point>()
  return {
    start: (tokenId, to) => {
      pending.set(tokenId, to)
    },
    confirm: (tokenId) => {
      pending.delete(tokenId)
    },
    fail: (tokenId) => {
      pending.delete(tokenId)
    },
    position: (t) => pending.get(t.id) ?? { x: t.x, y: t.y },
  }
}

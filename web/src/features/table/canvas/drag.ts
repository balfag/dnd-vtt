import type { Grid, Token } from '../../../shared/protocol'
import type { Point } from './camera'
import { snapTokenCenter } from './grid'

/** token.drag goes out at most every 50 ms (20 per second). */
export const DRAG_INTERVAL_MS = 50

const edgeEpsilon = 1e-6

interface Size {
  w: number
  h: number
}
interface Bounds {
  width: number
  height: number
}

/**
 * DragState is a token being dragged by this client. origin is the token centre at the start,
 * grab is the pointer there, current is the centre now (not snapped yet).
 */
export interface DragState {
  tokenId: string
  origin: Point
  grab: Point
  current: Point
  lastSentAt: number
}

/** canMove tells whether the viewer may move the token: the GM always, an owner while it is not locked. */
export function canMove(t: Token, me: { isGM: boolean; playerId: string | null }): boolean {
  if (me.isGM) return true
  return me.playerId !== null && t.owners.includes(me.playerId) && !t.locked
}

/** startDrag begins a drag; the first move is sent at once. */
export function startDrag(t: Token, pointerWorld: Point, now: number): DragState {
  const origin = { x: t.x, y: t.y }
  return {
    tokenId: t.id,
    origin,
    grab: pointerWorld,
    current: origin,
    lastSentAt: now - DRAG_INTERVAL_MS,
  }
}

function clamp(p: Point, bounds: Bounds): Point {
  return {
    x: Math.min(Math.max(p.x, 0), bounds.width),
    y: Math.min(Math.max(p.y, 0), bounds.height),
  }
}

function place(raw: Point, g: Grid, size: Size, snap: boolean, bounds: Bounds): Point {
  if (!snap) return raw
  // The edge itself belongs to the cell before it, so a snapped token stays on the scene.
  const inside = clamp(raw, {
    width: bounds.width - edgeEpsilon,
    height: bounds.height - edgeEpsilon,
  })
  return clamp(snapTokenCenter(g, inside, size), bounds)
}

/** moveDrag returns the new state and, when 50 ms passed since the last send, the position to send as token.drag. */
export function moveDrag(
  s: DragState,
  pointerWorld: Point,
  now: number,
  g: Grid,
  size: Size,
  snap: boolean,
  bounds: Bounds,
): { state: DragState; send: Point | null } {
  const current = clamp(
    {
      x: s.origin.x + pointerWorld.x - s.grab.x,
      y: s.origin.y + pointerWorld.y - s.grab.y,
    },
    bounds,
  )
  if (now - s.lastSentAt < DRAG_INTERVAL_MS) {
    return { state: { ...s, current }, send: null }
  }
  return {
    state: { ...s, current, lastSentAt: now },
    send: place(current, g, size, snap, bounds),
  }
}

/** endDrag is the final position for token.move; a drag that never moved ends where it began. */
export function endDrag(s: DragState, g: Grid, size: Size, snap: boolean, bounds: Bounds): Point {
  if (s.current.x === s.origin.x && s.current.y === s.origin.y) return s.origin
  return place(s.current, g, size, snap, bounds)
}

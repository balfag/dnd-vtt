import type { Grid, Token } from '../../../shared/protocol'
import { fit, pan, zoomAt, type Camera, type Point } from './camera'
import { canMove } from './drag'
import { snapTokenCenter } from './grid'

// Gestures to camera actions. Pure logic: the component feeds it events and applies the result.

export const ZOOM_STEP = 1.25
export const KEY_PAN_PX = 64

interface Size {
  width: number
  height: number
}

/** onWheel zooms around the pointer; a trackpad pinch arrives as a wheel event with ctrlKey. */
export function onWheel(c: Camera, e: { deltaY: number; ctrlKey: boolean; point: Point }): Camera {
  const factor = e.ctrlKey ? Math.exp(-e.deltaY / 100) : e.deltaY < 0 ? 1.1 : 1 / 1.1
  return zoomAt(c, e.point, factor)
}

/** zoomCenter zooms around the centre of the viewport (buttons and keys). */
export function zoomCenter(c: Camera, viewport: Size, factor: number): Camera {
  return zoomAt(c, { x: viewport.width / 2, y: viewport.height / 2 }, factor)
}

/** Gesture holds the pointers currently pressed that drive the camera. */
export interface Gesture {
  pointers: ReadonlyMap<number, Point>
}

export const noGesture: Gesture = { pointers: new Map() }

export function pointerDown(g: Gesture, id: number, p: Point): Gesture {
  const pointers = new Map(g.pointers)
  pointers.set(id, p)
  return { pointers }
}

export function pointerUp(g: Gesture, id: number): Gesture {
  if (!g.pointers.has(id)) return g
  const pointers = new Map(g.pointers)
  pointers.delete(id)
  return { pointers }
}

/**
 * pointerMove returns the next gesture and camera: one pointer pans, two pinch
 * (zoom by the change of distance around the midpoint, pan by the midpoint shift).
 * A pointer that is not pressed changes nothing.
 */
export function pointerMove(
  g: Gesture,
  c: Camera,
  id: number,
  p: Point,
): { gesture: Gesture; camera: Camera } {
  const prev = g.pointers.get(id)
  if (prev === undefined) return { gesture: g, camera: c }
  const pointers = new Map(g.pointers)
  pointers.set(id, p)
  const gesture = { pointers }

  if (g.pointers.size === 1) {
    return { gesture, camera: pan(c, p.x - prev.x, p.y - prev.y) }
  }
  if (g.pointers.size === 2) {
    const before = [...g.pointers.values()]
    const after = [...pointers.values()]
    const [a0, b0] = before
    const [a1, b1] = after
    if (a0 === undefined || b0 === undefined || a1 === undefined || b1 === undefined) {
      return { gesture, camera: c }
    }
    const mid0 = { x: (a0.x + b0.x) / 2, y: (a0.y + b0.y) / 2 }
    const mid1 = { x: (a1.x + b1.x) / 2, y: (a1.y + b1.y) / 2 }
    const d0 = Math.hypot(a0.x - b0.x, a0.y - b0.y)
    const d1 = Math.hypot(a1.x - b1.x, a1.y - b1.y)
    const zoomed = d0 > 0 && d1 > 0 ? zoomAt(c, mid0, d1 / d0) : c
    return { gesture, camera: pan(zoomed, mid1.x - mid0.x, mid1.y - mid0.y) }
  }
  return { gesture, camera: c }
}

/** onKey maps a key to the next camera, or null when the key is not a camera key. */
export function onKey(
  c: Camera,
  key: string,
  viewport: Size,
  scene: { width: number; height: number } | null,
): Camera | null {
  switch (key) {
    case '+':
    case '=':
      return zoomCenter(c, viewport, ZOOM_STEP)
    case '-':
      return zoomCenter(c, viewport, 1 / ZOOM_STEP)
    case '0':
      return scene === null ? null : fit(scene, viewport)
    case 'ArrowLeft':
      return pan(c, KEY_PAN_PX, 0)
    case 'ArrowRight':
      return pan(c, -KEY_PAN_PX, 0)
    case 'ArrowUp':
      return pan(c, 0, KEY_PAN_PX)
    case 'ArrowDown':
      return pan(c, 0, -KEY_PAN_PX)
    default:
      return null
  }
}

/**
 * cycleToken picks the next (or previous) token the viewer may move, in the order of z, then id,
 * wrapping around. With no selection, or a selection that is not in the list, it starts at the first (last).
 */
export function cycleToken(
  tokens: readonly Token[],
  selectedId: string | null,
  back: boolean,
  me: { isGM: boolean; playerId: string | null },
): string | null {
  const movable = tokens
    .filter((t) => canMove(t, me))
    .sort((a, b) => a.z - b.z || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  if (movable.length === 0) return null
  const at = movable.findIndex((t) => t.id === selectedId)
  const next =
    at < 0
      ? back
        ? movable.length - 1
        : 0
      : (at + (back ? -1 : 1) + movable.length) % movable.length
  return movable[next]?.id ?? null
}

const arrows: Record<string, Point> = {
  ArrowLeft: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 },
  ArrowUp: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 },
}

/**
 * stepPosition is the centre of the token one cell away in the arrow's direction, snapped
 * to the grid; null for other keys, and when the step would leave the scene or not move.
 */
export function stepPosition(
  t: Token,
  key: string,
  g: Grid,
  bounds: { width: number; height: number },
): Point | null {
  const dir = arrows[key]
  if (dir === undefined) return null
  const to = snapTokenCenter(g, { x: t.x + dir.x * g.sizePx, y: t.y + dir.y * g.sizePx }, t)
  if (to.x < 0 || to.y < 0 || to.x > bounds.width || to.y > bounds.height) return null
  return to.x === t.x && to.y === t.y ? null : to
}

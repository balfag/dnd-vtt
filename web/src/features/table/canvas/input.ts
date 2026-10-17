import { fit, pan, zoomAt, type Camera, type Point } from './camera'

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

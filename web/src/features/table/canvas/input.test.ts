import { describe, expect, it } from 'vitest'
import { screenToWorld, worldToScreen, type Camera } from './camera'
import {
  KEY_PAN_PX,
  noGesture,
  onKey,
  onWheel,
  pointerDown,
  pointerMove,
  pointerUp,
  zoomCenter,
} from './input'

const cam: Camera = { x: 50, y: 80, zoom: 1 }
const viewport = { width: 800, height: 600 }

describe('onWheel', () => {
  it('zooms in on wheel up and out on wheel down around the pointer', () => {
    const point = { x: 200, y: 100 }
    const up = onWheel(cam, { deltaY: -100, ctrlKey: false, point })
    const down = onWheel(cam, { deltaY: 100, ctrlKey: false, point })
    expect(up.zoom).toBeCloseTo(1.1, 9)
    expect(down.zoom).toBeCloseTo(1 / 1.1, 9)
    const before = screenToWorld(cam, point)
    const after = screenToWorld(up, point)
    expect(after.x).toBeCloseTo(before.x, 9)
    expect(after.y).toBeCloseTo(before.y, 9)
  })

  it('uses an exponential factor for a trackpad pinch', () => {
    const next = onWheel(cam, { deltaY: -20, ctrlKey: true, point: { x: 0, y: 0 } })
    expect(next.zoom).toBeCloseTo(Math.exp(0.2), 9)
    const smaller = onWheel(cam, { deltaY: 50, ctrlKey: true, point: { x: 0, y: 0 } })
    expect(smaller.zoom).toBeCloseTo(Math.exp(-0.5), 9)
  })
})

describe('drag', () => {
  it('pans by the pointer shift', () => {
    let g = pointerDown(noGesture, 1, { x: 100, y: 100 })
    const r = pointerMove(g, cam, 1, { x: 130, y: 90 })
    g = r.gesture
    const p = { x: 300, y: 300 }
    const shift = worldToScreen(r.camera, p)
    const start = worldToScreen(cam, p)
    expect(shift.x - start.x).toBeCloseTo(30, 9)
    expect(shift.y - start.y).toBeCloseTo(-10, 9)
    expect(r.camera.zoom).toBe(1)
    // The next move is relative to the last position.
    const r2 = pointerMove(g, r.camera, 1, { x: 130, y: 90 })
    expect(r2.camera).toEqual(r.camera)
  })

  it('ignores moves of a pointer that is not pressed', () => {
    const r = pointerMove(noGesture, cam, 7, { x: 1, y: 1 })
    expect(r.camera).toBe(cam)
  })

  it('stops after the pointer is released', () => {
    const g = pointerUp(pointerDown(noGesture, 1, { x: 0, y: 0 }), 1)
    expect(pointerMove(g, cam, 1, { x: 50, y: 50 }).camera).toBe(cam)
  })
})

describe('two-finger pinch', () => {
  it('scales by the distance change around the midpoint', () => {
    let g = pointerDown(noGesture, 1, { x: 100, y: 200 })
    g = pointerDown(g, 2, { x: 200, y: 200 })
    const mid = { x: 150, y: 200 }
    const worldBefore = screenToWorld(cam, mid)
    const r = pointerMove(g, cam, 2, { x: 300, y: 200 })
    // Distance 100 -> 200 and the midpoint moved from 150 to 200.
    expect(r.camera.zoom).toBeCloseTo(2, 9)
    const onScreen = worldToScreen(r.camera, worldBefore)
    expect(onScreen.x).toBeCloseTo(200, 9)
    expect(onScreen.y).toBeCloseTo(200, 9)
  })

  it('pans by the midpoint shift when the distance is unchanged', () => {
    let g = pointerDown(noGesture, 1, { x: 100, y: 100 })
    g = pointerDown(g, 2, { x: 200, y: 100 })
    const r1 = pointerMove(g, cam, 1, { x: 110, y: 120 })
    const r2 = pointerMove(r1.gesture, r1.camera, 2, { x: 210, y: 120 })
    expect(r2.camera.zoom).toBeCloseTo(1, 9)
    const p = { x: 400, y: 400 }
    const shift = worldToScreen(r2.camera, p)
    const start = worldToScreen(cam, p)
    expect(shift.x - start.x).toBeCloseTo(10, 9)
    expect(shift.y - start.y).toBeCloseTo(20, 9)
  })
})

describe('keys', () => {
  it('zooms in and out around the centre', () => {
    const centre = { x: 400, y: 300 }
    const before = screenToWorld(cam, centre)
    for (const key of ['+', '=']) {
      const next = onKey(cam, key, viewport, null)
      expect(next?.zoom).toBeCloseTo(1.25, 9)
      if (next === null) throw new Error('no camera')
      const after = screenToWorld(next, centre)
      expect(after.x).toBeCloseTo(before.x, 9)
      expect(after.y).toBeCloseTo(before.y, 9)
    }
    expect(onKey(cam, '-', viewport, null)?.zoom).toBeCloseTo(0.8, 9)
    expect(zoomCenter(cam, viewport, 2).zoom).toBe(2)
  })

  it('0 fits the scene when there is one', () => {
    const next = onKey(cam, '0', viewport, { width: 3000, height: 2000 })
    expect(next).not.toBeNull()
    expect(next?.zoom).toBeLessThan(1)
    expect(onKey(cam, '0', viewport, null)).toBeNull()
  })

  it('arrows pan by 64px and move the view, not the content', () => {
    const left = onKey(cam, 'ArrowLeft', viewport, null)
    expect(left?.x).toBeCloseTo(cam.x - KEY_PAN_PX, 9)
    const down = onKey(cam, 'ArrowDown', viewport, null)
    expect(down?.y).toBeCloseTo(cam.y + KEY_PAN_PX, 9)
    expect(onKey(cam, 'ArrowRight', viewport, null)?.x).toBeCloseTo(cam.x + KEY_PAN_PX, 9)
    expect(onKey(cam, 'ArrowUp', viewport, null)?.y).toBeCloseTo(cam.y - KEY_PAN_PX, 9)
  })

  it('returns null for other keys', () => {
    expect(onKey(cam, 'a', viewport, null)).toBeNull()
  })
})

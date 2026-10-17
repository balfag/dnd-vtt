import { describe, expect, it } from 'vitest'
import {
  MAX_ZOOM,
  MIN_ZOOM,
  fit,
  pan,
  screenToWorld,
  worldToScreen,
  zoomAt,
  type Camera,
} from './camera'

const cam: Camera = { x: 120, y: -40, zoom: 1.7 }

describe('camera', () => {
  it('worldToScreen and screenToWorld are inverse', () => {
    const p = { x: 333.5, y: -12.25 }
    const back = screenToWorld(cam, worldToScreen(cam, p))
    expect(back.x).toBeCloseTo(p.x, 9)
    expect(back.y).toBeCloseTo(p.y, 9)
  })

  it('zoomAt keeps the world point under the cursor', () => {
    const screen = { x: 300, y: 200 }
    const before = screenToWorld(cam, screen)
    for (const factor of [1.1, 1 / 1.1, 0.5, 2]) {
      const next = zoomAt(cam, screen, factor)
      const after = screenToWorld(next, screen)
      expect(Math.abs(after.x - before.x)).toBeLessThan(1e-9)
      expect(Math.abs(after.y - before.y)).toBeLessThan(1e-9)
    }
  })

  it('zoomAt clamps the zoom on both sides', () => {
    const screen = { x: 10, y: 10 }
    expect(zoomAt(cam, screen, 1000).zoom).toBe(MAX_ZOOM)
    expect(zoomAt(cam, screen, 0.0001).zoom).toBe(MIN_ZOOM)
  })

  it('zoomAt keeps the point fixed even when the zoom is clamped', () => {
    const screen = { x: 77, y: 31 }
    const before = screenToWorld(cam, screen)
    const after = screenToWorld(zoomAt(cam, screen, 1000), screen)
    expect(Math.abs(after.x - before.x)).toBeLessThan(1e-9)
    expect(Math.abs(after.y - before.y)).toBeLessThan(1e-9)
  })

  it('pan moves the content with the pointer', () => {
    const p = { x: 500, y: 500 }
    const before = worldToScreen(cam, p)
    const after = worldToScreen(pan(cam, 30, -20), p)
    expect(after.x - before.x).toBeCloseTo(30, 9)
    expect(after.y - before.y).toBeCloseTo(-20, 9)
  })

  it('fit shows a wide and tall scene inside the margin, centred', () => {
    const viewport = { width: 1000, height: 600 }
    for (const scene of [
      { width: 3000, height: 2000 },
      { width: 20000, height: 1000 },
      { width: 500, height: 9000 },
    ]) {
      const c = fit(scene, viewport)
      const tl = worldToScreen(c, { x: 0, y: 0 })
      const br = worldToScreen(c, { x: scene.width, y: scene.height })
      if (c.zoom > MIN_ZOOM) {
        expect(tl.x).toBeGreaterThanOrEqual(24 - 1e-9)
        expect(tl.y).toBeGreaterThanOrEqual(24 - 1e-9)
        expect(br.x).toBeLessThanOrEqual(viewport.width - 24 + 1e-9)
        expect(br.y).toBeLessThanOrEqual(viewport.height - 24 + 1e-9)
      }
      expect((tl.x + br.x) / 2).toBeCloseTo(viewport.width / 2, 9)
      expect((tl.y + br.y) / 2).toBeCloseTo(viewport.height / 2, 9)
    }
  })

  it('fit touches the margin on the limiting side', () => {
    const c = fit({ width: 3000, height: 2000 }, { width: 1440, height: 900 })
    const tl = worldToScreen(c, { x: 0, y: 0 })
    // 3000x2000 in 1440x900: the height limits, so the top edge sits on the margin.
    expect(tl.y).toBeCloseTo(24, 9)
  })

  it('fit clamps the zoom for a small scene', () => {
    expect(fit({ width: 10, height: 10 }, { width: 1000, height: 1000 }).zoom).toBe(MAX_ZOOM)
  })
})

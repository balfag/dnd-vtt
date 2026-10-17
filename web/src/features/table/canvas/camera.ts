export type Point = { x: number; y: number }

/** Camera: x, y is the world point at the viewport's top-left; screen = (world - {x,y}) * zoom. */
export interface Camera {
  x: number
  y: number
  zoom: number
}

export const MIN_ZOOM = 0.1
export const MAX_ZOOM = 4

const fitMargin = 24

function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom))
}

export function worldToScreen(c: Camera, p: Point): Point {
  return { x: (p.x - c.x) * c.zoom, y: (p.y - c.y) * c.zoom }
}

export function screenToWorld(c: Camera, p: Point): Point {
  return { x: p.x / c.zoom + c.x, y: p.y / c.zoom + c.y }
}

/** zoomAt keeps the world point under `screen` fixed; zoom is clamped to [MIN_ZOOM, MAX_ZOOM]. */
export function zoomAt(c: Camera, screen: Point, factor: number): Camera {
  const zoom = clampZoom(c.zoom * factor)
  const world = screenToWorld(c, screen)
  return { x: world.x - screen.x / zoom, y: world.y - screen.y / zoom, zoom }
}

export function pan(c: Camera, dxScreen: number, dyScreen: number): Camera {
  return { x: c.x - dxScreen / c.zoom, y: c.y - dyScreen / c.zoom, zoom: c.zoom }
}

/** fit shows the whole scene centred with a 24px margin, zoom clamped. */
export function fit(
  scene: { width: number; height: number },
  viewport: { width: number; height: number },
): Camera {
  const availW = Math.max(1, viewport.width - 2 * fitMargin)
  const availH = Math.max(1, viewport.height - 2 * fitMargin)
  const zoom = clampZoom(Math.min(availW / scene.width, availH / scene.height))
  return {
    x: scene.width / 2 - viewport.width / 2 / zoom,
    y: scene.height / 2 - viewport.height / 2 / zoom,
    zoom,
  }
}

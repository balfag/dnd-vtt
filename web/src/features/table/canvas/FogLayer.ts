import { Container, Graphics, RenderTexture, Sprite, Texture, type Application } from 'pixi.js'
import type { Fog, Shape } from '../../../shared/protocol'
import { worldToScreen, type Camera, type Point } from './camera'
import { brushStamps } from './fogShapes'

/** The fog mask never has a side longer than this, however big the scene is. */
const maxMaskSide = 4096
const playerAlpha = 1
const gmAlpha = 0.55
const outlineWidth = 1.5
const dashLen = 8
const dashGap = 5
const circleSegments = 48
const vertexRadius = 4

/** maskScale is how much smaller than the scene the mask texture is. */
export function maskScale(scene: { width: number; height: number }): number {
  return Math.min(1, maxMaskSide / Math.max(scene.width, scene.height, 1))
}

function drawShape(g: Graphics, s: Shape): void {
  switch (s.type) {
    case 'rect':
      g.rect(s.x, s.y, s.w, s.h)
      break
    case 'circle':
      g.circle(s.x, s.y, s.r)
      break
    case 'polygon':
      g.poly(s.points.flat())
      break
  }
}

interface FogSource {
  fog: Fog
  width: number
  height: number
}

/**
 * FogLayer draws the fog of a scene: a mask in a RenderTexture (the scene covered, then every
 * operation in order: hide fills, reveal erases) shown as a sprite in --slate. The mask is redrawn
 * only when the fog, the scene size or the viewer's role changes.
 */
export class FogLayer {
  readonly container = new Container()

  private readonly app: Application
  private readonly color: string
  private readonly sprite = new Sprite()
  private texture: RenderTexture | null = null
  private drawnFog: Fog | null = null
  private drawnWidth = 0
  private drawnHeight = 0

  constructor(app: Application, color: string) {
    this.app = app
    this.color = color
    this.sprite.tint = color
    this.sprite.eventMode = 'none'
    this.sprite.visible = false
    this.container.eventMode = 'none'
    this.container.addChild(this.sprite)
  }

  /** setFog shows the fog of the scene (null: no scene). It redraws the mask only if something changed. */
  setFog(source: FogSource | null, isGM: boolean): void {
    if (source === null || !source.fog.enabled) {
      this.sprite.visible = false
      this.drawnFog = null
      this.releaseTexture()
      return
    }
    this.sprite.alpha = isGM ? gmAlpha : playerAlpha
    const same =
      this.drawnFog === source.fog &&
      this.drawnWidth === source.width &&
      this.drawnHeight === source.height
    if (!same) this.redraw(source)
  }

  destroy(): void {
    this.releaseTexture()
  }

  private releaseTexture(): void {
    this.sprite.texture = Texture.EMPTY
    this.texture?.destroy(true)
    this.texture = null
  }

  private redraw(source: FogSource): void {
    const { fog, width, height } = source
    const scale = maskScale(source)
    const texW = Math.max(1, Math.round(width * scale))
    const texH = Math.max(1, Math.round(height * scale))
    if (this.texture === null || this.texture.width !== texW || this.texture.height !== texH) {
      this.releaseTexture()
      this.texture = RenderTexture.create({ width: texW, height: texH, resolution: 1 })
    }
    // Operations are applied in order, from scratch: the last one that holds a point decides.
    const mask = new Container()
    mask.scale.set(texW / width, texH / height)
    const cover = new Graphics().rect(0, 0, width, height).fill(0xffffff)
    mask.addChild(cover)
    for (const op of fog.ops) {
      const g = new Graphics()
      drawShape(g, op.shape)
      g.fill(0xffffff)
      if (op.op === 'reveal') g.blendMode = 'erase'
      mask.addChild(g)
    }
    this.app.renderer.render({
      container: mask,
      target: this.texture,
      clear: true,
      clearColor: [0, 0, 0, 0],
    })
    mask.destroy({ children: true })
    this.sprite.texture = this.texture
    this.sprite.tint = this.color
    this.sprite.width = width
    this.sprite.height = height
    this.sprite.visible = true
    this.drawnFog = fog
    this.drawnWidth = width
    this.drawnHeight = height
  }
}

/** FogPreview is what a fog tool is drawing now; points are in scene coordinates. */
export interface FogPreviewModel {
  shape: 'rect' | 'polygon' | 'brush'
  /** rect: its two corners; polygon: the corners so far; brush: the stroke. */
  points: Point[]
  /** The pointer, for the polygon's closing edge. */
  cursor: Point | null
  /** The brush radius in scene pixels. */
  radius: number
}

function dashedPath(g: Graphics, path: Point[], closed: boolean): void {
  const pts = closed ? [...path, ...path.slice(0, 1)] : path
  let carried = 0 // how far into the current dash-and-gap period the previous segment ended
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1]
    const b = pts[i]
    if (a === undefined || b === undefined) continue
    const len = Math.hypot(b.x - a.x, b.y - a.y)
    if (len === 0) continue
    let d = 0
    while (d < len) {
      const phase = (carried + d) % (dashLen + dashGap)
      if (phase < dashLen) {
        const end = Math.min(len, d + (dashLen - phase))
        g.moveTo(a.x + ((b.x - a.x) * d) / len, a.y + ((b.y - a.y) * d) / len)
        g.lineTo(a.x + ((b.x - a.x) * end) / len, a.y + ((b.y - a.y) * end) / len)
        d = end
      } else {
        d += dashLen + dashGap - phase
      }
    }
    carried = (carried + len) % (dashLen + dashGap)
  }
}

/** FogPreviewLayer draws the dashed outline of the shape being drawn, in screen pixels. */
export class FogPreviewLayer {
  readonly graphics = new Graphics()

  private readonly color: string

  constructor(color: string) {
    this.color = color
    this.graphics.eventMode = 'none'
  }

  /** draw shows the preview (null: clear it) under the camera. */
  draw(model: FogPreviewModel | null, camera: Camera): void {
    const g = this.graphics
    g.clear()
    if (model === null) return
    const screen = (p: Point) => worldToScreen(camera, p)
    switch (model.shape) {
      case 'rect': {
        const a = model.points[0]
        const b = model.points[1]
        if (a === undefined || b === undefined) break
        const tl = screen({ x: Math.min(a.x, b.x), y: Math.min(a.y, b.y) })
        const br = screen({ x: Math.max(a.x, b.x), y: Math.max(a.y, b.y) })
        dashedPath(g, [tl, { x: br.x, y: tl.y }, br, { x: tl.x, y: br.y }], true)
        break
      }
      case 'polygon': {
        const path = model.points.map(screen)
        if (model.cursor !== null) path.push(screen(model.cursor))
        dashedPath(g, path, path.length > 2)
        break
      }
      case 'brush': {
        for (const s of brushStamps(model.points, model.radius)) {
          if (s.type !== 'circle') continue
          const ring: Point[] = []
          for (let i = 0; i < circleSegments; i++) {
            const a = (i / circleSegments) * Math.PI * 2
            ring.push(screen({ x: s.x + s.r * Math.cos(a), y: s.y + s.r * Math.sin(a) }))
          }
          dashedPath(g, ring, true)
        }
        break
      }
    }
    g.stroke({ width: outlineWidth, color: this.color })
    if (model.shape === 'polygon') {
      // The first corner is a target: a click on it closes the polygon.
      const first = model.points[0]
      if (first !== undefined && model.points.length >= 3) {
        const p = screen(first)
        g.circle(p.x, p.y, vertexRadius).stroke({ width: outlineWidth, color: this.color })
      }
    }
  }
}

import { Container, Graphics } from 'pixi.js'
import type { Drawing, SceneView } from '../../../shared/protocol'
import { worldToScreen, type Camera, type Point } from './camera'
import { paintShape, TemplateCells, type DrawingColors } from './DrawingLayer'
import { Label, type Colors } from './TokenLayer'

/** RulerView is a ruler to show: the path through cell centres (world), the colour of its owner. */
export interface RulerView {
  key: string
  path: Point[]
  /** The line; --chalk for the viewer's own ruler. */
  color: string
  /** The colour of the text on the plaque: readable on --slate. */
  labelColor: string
  text: string
}

/** PingView is a ping to show. */
export interface PingView {
  id: string
  x: number
  y: number
  color: string
  /** Date.now() when it arrived. */
  at: number
}

/** OverlayModel is everything the layer needs to draw one frame of rulers, pings and the preview. */
export interface OverlayModel {
  scene: SceneView | null
  rulers: readonly RulerView[]
  pings: readonly PingView[]
  /** The drawing or template being drawn, not sent yet. */
  preview: Drawing | null
  reducedMotion: boolean
}

const rulerWidth = 2
const dashLen = 8
const dashGap = 5
const waypointRadius = 3.5
const pingMs = 1200
const pingLifeMs = 2000
const ringStaggerMs = 250
const ringFrom = 6
const ringTo = 42
const staticRings = [16, 28]
const labelOffset = 12

/** dashedPolyline strokes the polyline as dashes of a constant length along the whole path. */
function dashedPolyline(g: Graphics, pts: readonly Point[]): void {
  let draw = true
  let left = dashLen
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1]
    const b = pts[i]
    if (a === undefined || b === undefined) continue
    const len = Math.hypot(b.x - a.x, b.y - a.y)
    let at = 0
    while (at < len) {
      const step = Math.min(left, len - at)
      const x0 = a.x + ((b.x - a.x) * at) / len
      const y0 = a.y + ((b.y - a.y) * at) / len
      at += step
      left -= step
      if (draw) {
        g.moveTo(x0, y0).lineTo(a.x + ((b.x - a.x) * at) / len, a.y + ((b.y - a.y) * at) / len)
      }
      if (left <= 0) {
        draw = !draw
        left = draw ? dashLen : dashGap
      }
    }
  }
}

/** OverlayLayer draws what lies over the tokens and the fog: rulers, pings and the preview of a shape. */
export class OverlayLayer {
  readonly container = new Container()

  private readonly preview = new Graphics()
  private readonly rulers = new Graphics()
  private readonly pings = new Graphics()
  private readonly labels = new Map<string, Label>()
  private readonly cells = new TemplateCells()
  private model: OverlayModel | null = null
  private camera: Camera = { x: 0, y: 0, zoom: 1 }
  private view = { width: 0, height: 0 }
  private dirty = true

  constructor(
    private readonly colors: Colors & DrawingColors,
    private readonly now: () => number = Date.now,
  ) {
    for (const node of [this.container, this.preview, this.rulers, this.pings]) {
      node.eventMode = 'none'
    }
    this.container.addChild(this.preview, this.rulers, this.pings)
  }

  setCamera(c: Camera): void {
    this.camera = c
    this.dirty = true
  }

  setModel(m: OverlayModel): void {
    this.model = m
    this.dirty = true
  }

  /** tick redraws when something changed, and on every frame while a ping lives. */
  tick(view: { width: number; height: number }): void {
    if (view.width !== this.view.width || view.height !== this.view.height) {
      this.view = { width: view.width, height: view.height }
      this.dirty = true
    }
    const m = this.model
    if (m === null) return
    const animated = m.pings.length > 0
    if (!this.dirty && !animated) return
    this.dirty = animated
    this.draw(m)
  }

  destroy(): void {
    this.container.destroy({ children: true })
    this.labels.clear()
  }

  private draw(m: OverlayModel): void {
    this.drawPreview(m)
    this.drawRulers(m)
    this.drawPings(m)
  }

  private drawPreview(m: OverlayModel): void {
    const g = this.preview
    g.clear()
    const d = m.preview
    if (d === null || m.scene === null) {
      this.cells.keep(new Set())
      return
    }
    this.cells.keep(new Set([d.id]))
    const cells = d.kind === 'template' ? this.cells.of(d, m.scene) : []
    paintShape(g, this.camera, d, m.scene.grid, cells, this.colors, this.view)
  }

  private drawRulers(m: OverlayModel): void {
    const g = this.rulers
    g.clear()
    const live = new Set<string>()
    for (const r of m.rulers) {
      const pts = r.path.map((p) => worldToScreen(this.camera, p))
      const last = pts[pts.length - 1]
      if (last === undefined) continue
      live.add(r.key)
      // A slate underlay keeps the dashes readable on any map.
      dashedPolyline(g, pts)
      g.stroke({ width: rulerWidth + 2, color: this.colors.slate, alpha: 0.7 })
      dashedPolyline(g, pts)
      g.stroke({ width: rulerWidth, color: r.color })
      for (const p of pts) g.circle(p.x, p.y, waypointRadius)
      g.fill({ color: r.color })
      let label = this.labels.get(r.key)
      if (label === undefined) {
        label = new Label(this.colors)
        this.container.addChild(label.root)
        this.labels.set(r.key, label)
      }
      label.setFill(r.labelColor)
      label.set(r.text)
      label.root.visible = true
      const x = Math.min(last.x + labelOffset, this.view.width - label.width() - 4)
      const y = Math.min(last.y + labelOffset, this.view.height - label.height() - 4)
      label.root.position.set(Math.max(x, 4), Math.max(y, 4))
    }
    for (const [key, label] of this.labels) {
      if (live.has(key)) continue
      label.destroy()
      this.labels.delete(key)
    }
  }

  private drawPings(m: OverlayModel): void {
    const g = this.pings
    g.clear()
    const now = this.now()
    for (const ping of m.pings) {
      const age = now - ping.at
      if (age < 0 || age >= pingLifeMs) continue
      const c = worldToScreen(this.camera, ping)
      const fade = age > pingLifeMs - 300 ? (pingLifeMs - age) / 300 : 1
      const radii: { r: number; alpha: number }[] = []
      if (m.reducedMotion) {
        for (const r of staticRings) radii.push({ r, alpha: fade })
      } else {
        for (const [i, delay] of [0, ringStaggerMs].entries()) {
          const t = (age - delay) / pingMs
          if (t < 0 || t > 1) continue
          radii.push({
            r: ringFrom + (ringTo - ringFrom) * t,
            alpha: (1 - t) * (i === 0 ? 1 : 0.8),
          })
        }
      }
      for (const ring of radii) {
        g.circle(c.x, c.y, ring.r).stroke({
          width: 5,
          color: this.colors.slate,
          alpha: ring.alpha * 0.6,
        })
        g.circle(c.x, c.y, ring.r).stroke({ width: 3, color: ping.color, alpha: ring.alpha })
      }
      g.circle(c.x, c.y, 7).fill({ color: this.colors.slate, alpha: fade })
      g.circle(c.x, c.y, 5).fill({ color: ping.color, alpha: fade })
    }
  }
}

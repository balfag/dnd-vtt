import { Container, Graphics, Text } from 'pixi.js'
import type { Drawing, Grid, SceneView } from '../../../shared/protocol'
import { worldToScreen, type Camera } from './camera'
import { drawingBounds } from './drawings'
import { cellPolygon, type Cell } from './grid'
import { affectedCells, templateOutline } from './templates'
import { dashedRect } from './TokenLayer'

/** DrawingColors are the tokens of the palette the layer paints with. */
export interface DrawingColors {
  chalk: string
  verdigris: string
}

/** DrawingModel is everything the layer needs to draw one frame of drawings. */
export interface DrawingModel {
  scene: SceneView | null
  /** The drawings as they are shown now: a drawing being moved is already in its new place. */
  drawings: readonly Drawing[]
  selectedId: string | null
}

/** Lines are 3 px on the screen at any zoom. */
export const lineWidth = 3
const templateFillAlpha = 0.18
const cellFillAlpha = 0.12
const templateStrokeWidth = 2
const selectPad = 6

/** TemplateCells caches the cells of a template: the search is not free and the picture is redrawn often. */
export class TemplateCells {
  private readonly cache = new Map<string, { key: string; cells: Cell[] }>()

  of(d: Drawing, scene: SceneView): Cell[] {
    if (d.kind !== 'template') return []
    const g = scene.grid
    const key = `${JSON.stringify(d.data)}|${g.type}|${String(g.sizePx)}|${String(g.unitFt)}|${String(g.offsetX)}|${String(g.offsetY)}|${String(scene.width)}|${String(scene.height)}`
    const known = this.cache.get(d.id)
    if (known?.key === key) return known.cells
    const cells = affectedCells(d.data, g, { x: 0, y: 0, w: scene.width, h: scene.height })
    this.cache.set(d.id, { key, cells })
    return cells
  }

  /** keep forgets what is not among the ids. */
  keep(ids: ReadonlySet<string>): void {
    for (const id of this.cache.keys()) if (!ids.has(id)) this.cache.delete(id)
  }
}

/**
 * paintShape strokes (and for a template fills) a drawing in screen space. Text is not a shape:
 * the layer keeps a Text for it. view is the viewport; cells outside it are not painted.
 */
export function paintShape(
  g: Graphics,
  camera: Camera,
  d: Drawing,
  grid: Grid,
  cells: readonly Cell[],
  colors: DrawingColors,
  view: { width: number; height: number },
): void {
  const z = camera.zoom
  switch (d.kind) {
    case 'freehand': {
      const [first, ...rest] = d.data.points
      if (first === undefined) return
      const a = worldToScreen(camera, { x: first[0], y: first[1] })
      g.moveTo(a.x, a.y)
      for (const [x, y] of rest) {
        const p = worldToScreen(camera, { x, y })
        g.lineTo(p.x, p.y)
      }
      g.stroke({ width: lineWidth, color: d.color, cap: 'round', join: 'round' })
      return
    }
    case 'rect': {
      const p = worldToScreen(camera, d.data)
      g.rect(p.x, p.y, d.data.w * z, d.data.h * z).stroke({
        width: lineWidth,
        color: d.color,
        join: 'round',
      })
      return
    }
    case 'circle': {
      const p = worldToScreen(camera, d.data)
      g.circle(p.x, p.y, d.data.r * z).stroke({ width: lineWidth, color: d.color })
      return
    }
    case 'text':
      return
    case 'template': {
      const reach = grid.sizePx * z
      for (const cell of cells) {
        const poly = cellPolygon(grid, cell).map((v) => worldToScreen(camera, v))
        const first = poly[0]
        if (first === undefined) continue
        if (
          first.x < -reach ||
          first.y < -reach ||
          first.x > view.width + reach ||
          first.y > view.height + reach
        ) {
          continue
        }
        g.poly(poly.flatMap((v) => [v.x, v.y])).fill({
          color: colors.verdigris,
          alpha: cellFillAlpha,
        })
      }
      const outline = templateOutline(d.data, grid).map((v) => worldToScreen(camera, v))
      g.poly(outline.flatMap((v) => [v.x, v.y]))
        .fill({ color: colors.verdigris, alpha: templateFillAlpha })
        .stroke({ width: templateStrokeWidth, color: colors.verdigris, join: 'round' })
      return
    }
  }
}

/** DrawingLayer draws the drawings and the area templates of the scene in screen space. */
export class DrawingLayer {
  readonly container = new Container()

  private readonly shapes = new Graphics()
  private readonly selection = new Graphics()
  private readonly texts = new Map<string, Text>()
  private readonly cells = new TemplateCells()
  private model: DrawingModel | null = null
  private camera: Camera = { x: 0, y: 0, zoom: 1 }
  private view = { width: 0, height: 0 }
  private dirty = true

  private readonly colors: DrawingColors

  constructor(colors: DrawingColors) {
    this.colors = colors
    for (const node of [this.container, this.shapes, this.selection]) node.eventMode = 'none'
    this.container.addChild(this.shapes, this.selection)
  }

  setCamera(c: Camera): void {
    this.camera = c
    this.dirty = true
  }

  setModel(m: DrawingModel): void {
    this.model = m
    this.dirty = true
  }

  /** tick redraws when something changed; view is the size of the viewport. */
  tick(view: { width: number; height: number }): void {
    if (view.width !== this.view.width || view.height !== this.view.height) {
      this.view = { width: view.width, height: view.height }
      this.dirty = true
    }
    if (!this.dirty || this.model === null) return
    this.dirty = false
    this.draw(this.model)
  }

  destroy(): void {
    this.container.destroy({ children: true })
    this.texts.clear()
  }

  private draw(m: DrawingModel): void {
    this.shapes.clear()
    this.selection.clear()
    const scene = m.scene
    const live = new Set<string>()
    if (scene !== null) {
      for (const d of m.drawings) {
        live.add(d.id)
        if (d.kind === 'text') {
          this.drawText(d)
          continue
        }
        const cells = d.kind === 'template' ? this.cells.of(d, scene) : []
        paintShape(this.shapes, this.camera, d, scene.grid, cells, this.colors, this.view)
      }
      const picked = m.drawings.find((d) => d.id === m.selectedId)
      if (picked !== undefined) this.drawSelection(picked, scene.grid)
    }
    this.cells.keep(live)
    for (const [id, text] of this.texts) {
      if (live.has(id)) continue
      text.destroy()
      this.texts.delete(id)
    }
  }

  private drawText(d: Drawing): void {
    if (d.kind !== 'text') return
    let text = this.texts.get(d.id)
    if (text === undefined) {
      text = new Text({ text: '', style: { fontFamily: 'Alegreya Sans' } })
      text.eventMode = 'none'
      this.container.addChild(text)
      this.texts.set(d.id, text)
    }
    const p = worldToScreen(this.camera, d.data)
    text.text = d.data.text
    text.style.fill = d.color
    text.style.fontSize = Math.max(1, d.data.size * this.camera.zoom)
    text.position.set(p.x, p.y)
  }

  private drawSelection(d: Drawing, grid: Grid): void {
    const b = drawingBounds(d, grid)
    const p = worldToScreen(this.camera, b)
    dashedRect(
      this.selection,
      p.x - selectPad,
      p.y - selectPad,
      b.w * this.camera.zoom + selectPad * 2,
      b.h * this.camera.zoom + selectPad * 2,
    )
    this.selection.stroke({ width: 1.5, color: this.colors.chalk })
  }
}

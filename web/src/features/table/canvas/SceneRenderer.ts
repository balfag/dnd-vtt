import { Application, Container, Graphics, Sprite, Texture } from 'pixi.js'
import type { AssetRef, Grid, SceneView } from '../../../shared/protocol'
import { worldToScreen, screenToWorld, type Camera } from './camera'
import { clipSegment, hexEdges, intersect, squareLines } from './grid'
import { tileRects } from './tiles'

const maxTileSide = 4096
const gridAlpha = 0.28
const edgeAlpha = 0.35
// Below this cell size on the screen the grid is a haze of lines, and drawing it is wasted work.
const minCellScreenPx = 4

export interface SceneLayers {
  background: Container
  grid: Container
  edge: Container
}

function cssColor(host: HTMLElement, name: string, fallback: string): string {
  const value = getComputedStyle(host).getPropertyValue(name).trim()
  return value === '' ? fallback : value
}

/** SceneRenderer draws the scene surface with PixiJS: background, grid and the scene edge. */
export class SceneRenderer {
  readonly layers: SceneLayers

  private readonly app: Application
  private readonly boardColor: string
  private readonly fogColor: string
  private readonly board = new Graphics()
  private readonly gridShape = new Graphics()
  private readonly edgeShape = new Graphics()
  private readonly resizeObserver: ResizeObserver
  private readonly maxTile: number
  private scene: SceneView | null = null
  private camera: Camera = { x: 0, y: 0, zoom: 1 }
  private gridDirty = false
  private edgeDirty = false
  private loadId = 0
  private destroyed = false

  private constructor(app: Application, host: HTMLElement) {
    this.app = app
    this.boardColor = cssColor(host, '--board', '#2e3d49')
    this.fogColor = cssColor(host, '--fog', '#9aa8b3')
    this.layers = { background: new Container(), grid: new Container(), edge: new Container() }
    this.layers.background.addChild(this.board)
    this.layers.grid.addChild(this.gridShape)
    this.layers.edge.addChild(this.edgeShape)
    app.stage.addChild(this.layers.background, this.layers.grid, this.layers.edge)

    const renderer = app.renderer
    let maxSide = maxTileSide
    if ('gl' in renderer) {
      const reported: unknown = renderer.gl.getParameter(renderer.gl.MAX_TEXTURE_SIZE)
      if (typeof reported === 'number' && reported > 0) maxSide = Math.min(reported, maxTileSide)
    }
    this.maxTile = maxSide

    app.ticker.add(() => {
      this.flush()
    })
    renderer.on('resize', () => {
      this.gridDirty = true
      this.edgeDirty = true
    })
    this.resizeObserver = new ResizeObserver(() => {
      if (!this.destroyed) app.resize()
    })
    this.resizeObserver.observe(host)
  }

  /** create initialises PixiJS in host (resizeTo host, background --slate, antialias, autoDensity). */
  static async create(host: HTMLElement): Promise<SceneRenderer> {
    const app = new Application()
    await app.init({
      resizeTo: host,
      background: cssColor(host, '--slate', '#23303a'),
      antialias: true,
      autoDensity: true,
      resolution: window.devicePixelRatio,
    })
    host.appendChild(app.canvas)
    return new SceneRenderer(app, host)
  }

  viewport(): { width: number; height: number } {
    return { width: this.app.screen.width, height: this.app.screen.height }
  }

  setCamera(c: Camera): void {
    this.camera = c
    this.applyCamera()
    this.gridDirty = true
    this.edgeDirty = true
  }

  /** setScene shows the scene; the same scene id updates only what changed. */
  setScene(scene: SceneView | null): void {
    const prev = this.scene
    this.scene = scene
    if (scene === null) {
      this.loadId++
      this.clearBackground()
      this.board.clear()
      this.gridShape.clear()
      this.edgeShape.clear()
      return
    }
    const sameScene = prev !== null && prev.id === scene.id
    if (!sameScene || prev.width !== scene.width || prev.height !== scene.height) {
      this.board.clear().rect(0, 0, scene.width, scene.height).fill(this.boardColor)
      this.edgeDirty = true
    }
    if (!sameScene || !sameGrid(prev.grid, scene.grid)) this.gridDirty = true
    if (!sameScene || prev.background?.assetId !== scene.background?.assetId) {
      this.loadId++
      this.clearBackground()
      if (scene.background !== null) void this.loadBackground(scene.background, this.loadId)
    }
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.loadId++
    this.resizeObserver.disconnect()
    this.app.destroy(true, { children: true, texture: true })
  }

  private applyCamera(): void {
    const { x, y, zoom } = this.camera
    const bg = this.layers.background
    bg.position.set(-x * zoom, -y * zoom)
    bg.scale.set(zoom)
  }

  /** flush redraws the screen-space lines at most once per frame. */
  private flush(): void {
    if (this.destroyed || this.scene === null) return
    if (this.gridDirty) {
      this.gridDirty = false
      this.drawGrid(this.scene)
    }
    if (this.edgeDirty) {
      this.edgeDirty = false
      this.drawEdge(this.scene)
    }
  }

  private drawEdge(scene: SceneView): void {
    const tl = worldToScreen(this.camera, { x: 0, y: 0 })
    this.edgeShape
      .clear()
      .rect(tl.x, tl.y, scene.width * this.camera.zoom, scene.height * this.camera.zoom)
      .stroke({ width: 1, color: this.fogColor, alpha: edgeAlpha })
  }

  // The grid is drawn in screen coordinates, so its lines stay 1px at any zoom, and only
  // inside the scene: the part of the view that shows the scene.
  private drawGrid(scene: SceneView): void {
    const g = this.gridShape
    g.clear()
    const grid = scene.grid
    if (!grid.visible || grid.sizePx <= 0 || grid.sizePx * this.camera.zoom < minCellScreenPx) {
      return
    }
    const view = this.viewport()
    const topLeft = screenToWorld(this.camera, { x: 0, y: 0 })
    const shown = intersect(
      {
        x: topLeft.x,
        y: topLeft.y,
        w: view.width / this.camera.zoom,
        h: view.height / this.camera.zoom,
      },
      { x: 0, y: 0, w: scene.width, h: scene.height },
    )
    if (shown === null) return
    for (const [a, b] of grid.type === 'square'
      ? squareLines(grid, shown)
      : hexEdges(grid, shown)) {
      const clipped = clipSegment(a, b, shown)
      if (clipped === null) continue
      const from = worldToScreen(this.camera, clipped[0])
      const to = worldToScreen(this.camera, clipped[1])
      g.moveTo(from.x, from.y).lineTo(to.x, to.y)
    }
    g.stroke({ width: 1, color: grid.color, alpha: gridAlpha })
  }
  private clearBackground(): void {
    const bg = this.layers.background
    for (const child of bg.removeChildren()) {
      if (child !== this.board) child.destroy({ texture: true })
    }
    bg.addChild(this.board)
  }

  private async loadBackground(ref: AssetRef, id: number): Promise<void> {
    const sprites: Sprite[] = []
    const discard = () => {
      for (const s of sprites) s.destroy({ texture: true })
    }
    try {
      const res = await fetch(ref.url, { credentials: 'same-origin' })
      if (!res.ok) throw new Error(`HTTP ${String(res.status)}`)
      const blob = await res.blob()
      const size = await imageSize(blob)
      for (const t of tileRects(size.width, size.height, this.maxTile)) {
        if (id !== this.loadId) {
          discard()
          return
        }
        const bitmap = await createImageBitmap(blob, t.x, t.y, t.w, t.h)
        const sprite = new Sprite(Texture.from(bitmap))
        sprite.position.set(t.x, t.y)
        sprites.push(sprite)
      }
      if (id !== this.loadId) {
        discard()
        return
      }
      this.layers.background.addChild(...sprites)
    } catch (err) {
      discard()
      console.warn(`scene: cannot load the background ${ref.assetId}`, err)
    }
  }
}

function sameGrid(a: Grid, b: Grid): boolean {
  return (
    a.type === b.type &&
    a.sizePx === b.sizePx &&
    a.offsetX === b.offsetX &&
    a.offsetY === b.offsetY &&
    a.color === b.color &&
    a.visible === b.visible
  )
}

/** imageSize reads the natural size of an image without decoding it into a bitmap. */
function imageSize(blob: Blob): Promise<{ width: number; height: number }> {
  const url = URL.createObjectURL(blob)
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve({ width: img.naturalWidth, height: img.naturalHeight })
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('cannot read the image size'))
    }
    img.src = url
  })
}

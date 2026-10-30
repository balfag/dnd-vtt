import 'pixi.js/unsafe-eval'
import { Application, Container, Graphics, Sprite, Texture } from 'pixi.js'
import type { AssetRef, Grid, SceneView } from '../../../shared/protocol'
import { worldToScreen, screenToWorld, type Camera } from './camera'
import { clipSegment, hexEdges, intersect, squareLines } from './grid'
import { DrawingLayer, type DrawingModel } from './DrawingLayer'
import { FogLayer, FogPreviewLayer, type FogPreviewModel } from './FogLayer'
import { OverlayLayer, type OverlayModel } from './OverlayLayer'
import { tileRects } from './tiles'
import { TokenLayer, type TokenModel } from './TokenLayer'

const maxTileSide = 4096
const gridAlpha = 0.28
const edgeAlpha = 0.35
// Below this cell size on the screen the grid is a haze of lines, and drawing it is wasted work.
const minCellScreenPx = 4

export interface SceneLayers {
  background: Container
  mapTokens: Container
  grid: Container
  drawings: Container
  tokens: Container
  gmTokens: Container
  fog: Container
  edge: Container
  overlay: Container
  /** Rulers, pings and the preview of a shape: over everything. */
  tools: Container
}

function cssColor(host: HTMLElement, name: string, fallback: string): string {
  const value = getComputedStyle(host).getPropertyValue(name).trim()
  return value === '' ? fallback : value
}

/** SceneRenderer draws the scene with PixiJS: background, tokens, grid and the scene edge. */
export class SceneRenderer {
  readonly layers: SceneLayers

  private readonly app: Application
  private readonly boardColor: string
  private readonly fogColor: string
  private readonly slateColor: string
  private readonly board = new Graphics()
  private readonly gridShape = new Graphics()
  private readonly edgeShape = new Graphics()
  private readonly tokenLayer: TokenLayer
  private readonly fogLayer: FogLayer
  private readonly drawingLayer: DrawingLayer
  private readonly overlayLayer: OverlayLayer
  private readonly fogPreview: FogPreviewLayer
  private previewModel: FogPreviewModel | null = null
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
    this.slateColor = cssColor(host, '--slate', '#23303a')
    this.tokenLayer = new TokenLayer(app, {
      chalk: cssColor(host, '--chalk', '#ece7db'),
      slate: this.slateColor,
      fog: this.fogColor,
      line: cssColor(host, '--line', '#3a4a57'),
      verdigris: cssColor(host, '--verdigris', '#4fa58e'),
      seal: cssColor(host, '--seal', '#c25468'),
    })
    this.fogLayer = new FogLayer(app, this.slateColor)
    const palette = {
      chalk: cssColor(host, '--chalk', '#ece7db'),
      slate: this.slateColor,
      fog: this.fogColor,
      line: cssColor(host, '--line', '#3a4a57'),
      verdigris: cssColor(host, '--verdigris', '#4fa58e'),
      seal: cssColor(host, '--seal', '#c25468'),
    }
    this.drawingLayer = new DrawingLayer(palette)
    this.overlayLayer = new OverlayLayer(palette)
    this.fogPreview = new FogPreviewLayer(cssColor(host, '--brass', '#c9a34e'))
    this.layers = {
      background: new Container(),
      mapTokens: this.tokenLayer.layers.map,
      grid: new Container(),
      drawings: this.drawingLayer.container,
      tokens: this.tokenLayer.layers.tokens,
      gmTokens: this.tokenLayer.layers.gm,
      fog: this.fogLayer.container,
      edge: new Container(),
      overlay: this.tokenLayer.layers.overlay,
      tools: this.overlayLayer.container,
    }
    this.layers.background.addChild(this.board)
    this.layers.grid.addChild(this.gridShape)
    this.layers.edge.addChild(this.edgeShape, this.fogPreview.graphics)
    app.stage.addChild(
      this.layers.background,
      this.layers.mapTokens,
      this.layers.grid,
      this.layers.drawings,
      this.layers.tokens,
      this.layers.gmTokens,
      this.layers.fog,
      this.layers.edge,
      this.layers.overlay,
      this.layers.tools,
    )

    const renderer = app.renderer
    let maxSide = maxTileSide
    if ('gl' in renderer) {
      const reported: unknown = renderer.gl.getParameter(renderer.gl.MAX_TEXTURE_SIZE)
      if (typeof reported === 'number' && reported > 0) maxSide = Math.min(reported, maxTileSide)
    }
    this.maxTile = maxSide

    app.ticker.add((ticker) => {
      this.flush()
      if (this.destroyed) return
      this.tokenLayer.tick(ticker.deltaMS)
      this.drawingLayer.tick(this.app.screen)
      this.overlayLayer.tick(this.app.screen)
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
    this.fogPreview.draw(this.previewModel, c)
    this.tokenLayer.setCamera(c)
    this.drawingLayer.setCamera(c)
    this.overlayLayer.setCamera(c)
    this.gridDirty = true
    this.edgeDirty = true
  }

  /** setTokens gives the token layer what to draw; call it whenever the tokens, selection or drag change. */
  setTokens(model: TokenModel): void {
    if (!this.destroyed) this.tokenLayer.setModel(model)
  }

  /** setDrawings gives the drawing layer what to draw; call it whenever the drawings or the selection change. */
  setDrawings(model: DrawingModel): void {
    if (!this.destroyed) this.drawingLayer.setModel(model)
  }

  /** setOverlay gives the overlay what to draw: rulers, pings and the preview of a shape. */
  setOverlay(model: OverlayModel): void {
    if (!this.destroyed) this.overlayLayer.setModel(model)
  }

  /** setFog shows the fog of the scene, opaque for a player and see-through for the GM. */
  setFog(scene: SceneView | null, isGM: boolean): void {
    if (!this.destroyed) this.fogLayer.setFog(scene, isGM)
  }

  /** setFogPreview shows the outline of the fog shape being drawn (null: nothing). */
  setFogPreview(model: FogPreviewModel | null): void {
    this.previewModel = model
    if (!this.destroyed) this.fogPreview.draw(model, this.camera)
  }

  /** tokenAt is the id of the topmost token under the point (viewport coordinates), or null. */
  tokenAt(p: { x: number; y: number }): string | null {
    return this.destroyed ? null : this.tokenLayer.tokenAt(p)
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
    this.tokenLayer.destroy()
    this.drawingLayer.destroy()
    this.overlayLayer.destroy()
    this.fogLayer.destroy()
    this.app.destroy(true, { children: true, texture: true })
  }

  private applyCamera(): void {
    const { x, y, zoom } = this.camera
    const bg = this.layers.background
    bg.position.set(-x * zoom, -y * zoom)
    bg.scale.set(zoom)
    const fog = this.layers.fog
    fog.position.set(-x * zoom, -y * zoom)
    fog.scale.set(zoom)
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

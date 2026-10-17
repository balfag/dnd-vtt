import { Container, Ellipse, Graphics, Sprite, Text, Texture, type Application } from 'pixi.js'
import type { AssetRef, PlayerInfo, SceneView, Token } from '../../../shared/protocol'
import { worldToScreen, type Camera, type Point } from './camera'
import { tokenStyle, type TokenStyle } from './tokenStyle'

/** Where a token is drawn; smooth tokens glide there (100 ms) instead of jumping. */
export interface TokenPlacement {
  x: number
  y: number
  smooth: boolean
}

/** TokenModel is everything the layer needs to draw one frame of tokens. */
export interface TokenModel {
  scene: SceneView | null
  players: PlayerInfo[]
  isGM: boolean
  /** By token id; a token without a placement stays where the server has it. */
  placements: ReadonlyMap<string, TokenPlacement>
  selectedId: string | null
  hoverId: string | null
  /** The start of the drag in progress: the token drawn faintly where it was. */
  ghost: { token: Token; x: number; y: number } | null
  /** The distance plaque next to the pointer (screen coordinates). */
  distance: { at: Point; text: string } | null
  reducedMotion: boolean
}

export interface TokenLayers {
  map: Container
  tokens: Container
  gm: Container
  overlay: Container
}

interface Colors {
  chalk: string
  slate: string
  line: string
  verdigris: string
  seal: string
}

const glideMs = 100
// The glide is exponential; with this time constant it covers ~95% of the way in glideMs.
const glideTau = glideMs / 3
const ghostAlpha = 0.35
const letterFont = 64
const labelFont = 14
const labelPadX = 6
const labelPadY = 2
const labelGap = 4
const barHeight = 4
const barGap = 2
const dashLen = 8
const dashGap = 5
const selectPad = 5

function hit(node: Container): void {
  node.eventMode = 'none'
}

/** dashedEllipse strokes an ellipse around (0, 0) as separate dashes. */
function dashedEllipse(g: Graphics, rx: number, ry: number): void {
  const perimeter = Math.PI * (3 * (rx + ry) - Math.sqrt((3 * rx + ry) * (rx + 3 * ry)))
  const count = Math.round(perimeter / (dashLen + dashGap))
  if (count < 3) {
    g.ellipse(0, 0, rx, ry)
    return
  }
  const step = (Math.PI * 2) / count
  const span = (step * dashLen) / (dashLen + dashGap)
  const pieces = Math.max(2, Math.ceil((span * Math.max(rx, ry)) / 4))
  for (let i = 0; i < count; i++) {
    const a0 = i * step
    g.moveTo(rx * Math.cos(a0), ry * Math.sin(a0))
    for (let j = 1; j <= pieces; j++) {
      const a = a0 + (span * j) / pieces
      g.lineTo(rx * Math.cos(a), ry * Math.sin(a))
    }
  }
}

/** dashedRect strokes the rectangle (x, y, w, h) as dashes. */
function dashedRect(g: Graphics, x: number, y: number, w: number, h: number): void {
  const sides: [number, number, number, number][] = [
    [x, y, x + w, y],
    [x + w, y, x + w, y + h],
    [x + w, y + h, x, y + h],
    [x, y + h, x, y],
  ]
  for (const [x0, y0, x1, y1] of sides) {
    const len = Math.hypot(x1 - x0, y1 - y0)
    for (let d = 0; d < len; d += dashLen + dashGap) {
      const e = Math.min(d + dashLen, len)
      g.moveTo(x0 + ((x1 - x0) * d) / len, y0 + ((y1 - y0) * d) / len)
      g.lineTo(x0 + ((x1 - x0) * e) / len, y0 + ((y1 - y0) * e) / len)
    }
  }
}

/** A plaque: rounded slate background and a line of text that does not scale with the zoom. */
class Label {
  readonly root = new Container()
  private readonly bg = new Graphics()
  private readonly text: Text
  private shown = ''

  private readonly colors: Colors

  constructor(colors: Colors) {
    this.colors = colors
    this.text = new Text({
      text: '',
      style: { fontFamily: 'Alegreya Sans', fontSize: labelFont, fill: colors.chalk },
    })
    this.text.position.set(labelPadX, labelPadY)
    this.root.addChild(this.bg, this.text)
    hit(this.root)
    this.root.visible = false
  }

  width(): number {
    return this.text.width + labelPadX * 2
  }

  height(): number {
    return this.text.height + labelPadY * 2
  }

  set(text: string): void {
    if (text === this.shown) return
    this.shown = text
    this.text.text = text
    this.bg
      .clear()
      .roundRect(0, 0, this.width(), this.height(), 4)
      .fill(this.colors.slate)
      .stroke({ width: 1, color: this.colors.line })
  }

  destroy(): void {
    this.root.destroy({ children: true })
  }
}

/** TokenView draws one token: body, image, ring, letter, name and bars, around the root's origin. */
class TokenView {
  readonly root = new Container()
  private readonly fill = new Graphics()
  private readonly art = new Container()
  private readonly mask = new Graphics()
  private sprite: Sprite | null = null
  private spriteAsset: string | null = null
  private readonly ring = new Graphics()
  private readonly letter: Text
  private readonly name: Label
  private readonly bars = new Graphics()

  /** Where the token is now, world coordinates; glides toward its placement. */
  x = 0
  y = 0

  readonly id: string
  private readonly colors: Colors

  constructor(id: string, colors: Colors, interactive: boolean) {
    this.id = id
    this.colors = colors
    this.letter = new Text({
      text: '',
      style: {
        fontFamily: 'Alegreya',
        fontWeight: '700',
        fontSize: letterFont,
        fill: colors.chalk,
      },
    })
    this.letter.anchor.set(0.5)
    this.name = new Label(colors)
    this.art.addChild(this.mask)
    this.root.addChild(this.fill, this.art, this.ring, this.letter, this.bars, this.name.root)
    this.root.label = `token:${id}`
    if (interactive) this.root.eventMode = 'static'
    else hit(this.root)
    for (const child of [this.fill, this.art, this.ring, this.letter, this.bars]) hit(child)
  }

  draw(
    t: Token,
    style: TokenStyle,
    sizePx: number,
    camera: Camera,
    opts: {
      texture: Texture | null
      showName: boolean
      withBars: boolean
      alpha: number
    },
  ): void {
    const c = worldToScreen(camera, { x: this.x, y: this.y })
    const rx = (t.w * sizePx * camera.zoom) / 2
    const ry = (t.h * sizePx * camera.zoom) / 2
    this.root.position.set(c.x, c.y)
    this.root.alpha = opts.alpha * style.alpha
    this.root.hitArea = new Ellipse(0, 0, rx, ry)

    this.fill.clear().ellipse(0, 0, rx, ry).fill(style.fill)

    this.mask.clear().ellipse(0, 0, rx, ry).fill(0xffffff)
    this.drawArt(t, opts.texture, rx, ry)

    const sw = style.strokeWidth
    this.ring.clear()
    if (style.dashed) dashedEllipse(this.ring, Math.max(rx - sw / 2, 1), Math.max(ry - sw / 2, 1))
    else this.ring.ellipse(0, 0, Math.max(rx - sw / 2, 1), Math.max(ry - sw / 2, 1))
    this.ring.stroke({ width: sw, color: style.stroke })

    const showLetter = opts.texture === null && style.letter !== ''
    this.letter.visible = showLetter
    if (showLetter) {
      this.letter.text = style.letter
      this.letter.scale.set((ry * 2 * 0.45) / letterFont)
    }

    this.name.root.visible = opts.showName
    if (opts.showName) {
      this.name.set(t.name)
      this.name.root.position.set(-this.name.width() / 2, ry + labelGap)
    }

    this.bars.clear()
    if (opts.withBars) {
      const width = rx * 2
      t.bars.forEach((bar, i) => {
        const y = -ry - barGap - (i + 1) * barHeight - i * barGap
        const ratio = bar.max > 0 ? Math.min(Math.max(bar.value / bar.max, 0), 1) : 0
        this.bars.rect(-rx, y, width, barHeight).fill({ color: this.colors.slate, alpha: 0.8 })
        if (ratio > 0) {
          this.bars
            .rect(-rx, y, width * ratio, barHeight)
            .fill(ratio <= 0.25 ? this.colors.seal : this.colors.verdigris)
        }
      })
    }
  }

  private drawArt(t: Token, texture: Texture | null, rx: number, ry: number): void {
    if (texture === null) {
      this.sprite?.destroy()
      this.sprite = null
      this.spriteAsset = null
      this.art.visible = false
      return
    }
    this.art.visible = true
    const assetId = t.image?.assetId ?? null
    if (this.sprite === null || this.spriteAsset !== assetId || this.sprite.texture !== texture) {
      this.sprite?.destroy()
      this.sprite = new Sprite(texture)
      this.sprite.anchor.set(0.5)
      this.sprite.mask = this.mask
      hit(this.sprite)
      this.art.addChildAt(this.sprite, 0)
      this.spriteAsset = assetId
    }
    // The picture covers the ellipse; the mask cuts it to the ellipse.
    const scale = Math.max((rx * 2) / texture.width, (ry * 2) / texture.height)
    this.sprite.scale.set(scale)
    this.sprite.rotation = (t.rotation * Math.PI) / 180
  }

  destroy(): void {
    // Textures are shared and owned by the layer.
    this.sprite?.destroy({ texture: false })
    this.sprite = null
    this.root.destroy({ children: true })
  }
}

/** TokenLayer draws the tokens of the scene on PixiJS layers, in screen space (lines and text stay crisp). */
export class TokenLayer {
  readonly layers: TokenLayers

  private readonly app: Application
  private readonly colors: Colors
  private readonly views = new Map<string, TokenView>()
  private readonly textures = new Map<string, Texture | null>()
  private readonly loading = new Set<string>()
  private readonly selection = new Graphics()
  private readonly ghostView: TokenView
  private readonly distanceLabel: Label
  private model: TokenModel | null = null
  private camera: Camera = { x: 0, y: 0, zoom: 1 }
  private dirty = true
  private destroyed = false

  constructor(app: Application, colors: Colors) {
    this.app = app
    this.colors = colors
    this.layers = {
      map: new Container(),
      tokens: new Container(),
      gm: new Container(),
      overlay: new Container(),
    }
    for (const layer of [this.layers.map, this.layers.tokens, this.layers.gm]) {
      layer.sortableChildren = true
    }
    this.ghostView = new TokenView('ghost', colors, false)
    this.ghostView.root.visible = false
    this.distanceLabel = new Label(colors)
    hit(this.selection)
    hit(this.layers.overlay)
    this.layers.overlay.addChild(this.ghostView.root, this.selection, this.distanceLabel.root)
  }

  setCamera(c: Camera): void {
    this.camera = c
    this.dirty = true
  }

  setModel(m: TokenModel): void {
    const prev = this.model
    this.model = m
    this.dirty = true
    if (prev?.scene?.id !== m.scene?.id) this.dropViews()
    const tokens = m.scene?.tokens ?? []
    const live = new Set<string>()
    const used = new Set<string>()
    const ordered = [...tokens].sort(
      (a, b) => a.z - b.z || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0),
    )
    ordered.forEach((t, index) => {
      live.add(t.id)
      if (t.image !== null) {
        used.add(t.image.assetId)
        this.requestTexture(t.image)
      }
      let view = this.views.get(t.id)
      if (view === undefined) {
        view = new TokenView(t.id, this.colors, true)
        const at = m.placements.get(t.id) ?? t
        view.x = at.x
        view.y = at.y
        this.views.set(t.id, view)
      }
      const layer = this.layerOf(t)
      if (view.root.parent !== layer) layer.addChild(view.root)
      view.root.zIndex = index
    })
    for (const [id, view] of this.views) {
      if (live.has(id)) continue
      view.destroy()
      this.views.delete(id)
    }
    const ghostImage = m.ghost?.token.image ?? null
    if (ghostImage !== null) {
      used.add(ghostImage.assetId)
      this.requestTexture(ghostImage)
    }
    this.releaseTextures(used)
  }

  /** tick moves gliding tokens and redraws when something changed; dtMs is the frame time. */
  tick(dtMs: number): void {
    const m = this.model
    if (this.destroyed || m === null) return
    for (const t of m.scene?.tokens ?? []) {
      const view = this.views.get(t.id)
      if (view === undefined) continue
      const target = m.placements.get(t.id) ?? t
      const smooth = (m.placements.get(t.id)?.smooth ?? false) && !m.reducedMotion
      if (view.x === target.x && view.y === target.y) continue
      this.dirty = true
      if (!smooth) {
        view.x = target.x
        view.y = target.y
        continue
      }
      const k = 1 - Math.exp(-dtMs / glideTau)
      view.x += (target.x - view.x) * k
      view.y += (target.y - view.y) * k
      if (Math.abs(target.x - view.x) < 0.1 && Math.abs(target.y - view.y) < 0.1) {
        view.x = target.x
        view.y = target.y
      }
    }
    if (!this.dirty) return
    this.dirty = false
    this.draw(m)
  }

  /** tokenAt returns the id of the topmost token under the screen point, or null. */
  tokenAt(screen: Point): string | null {
    let node: Container | null = this.app.renderer.events.rootBoundary.hitTest(screen.x, screen.y)
    while (node !== null) {
      if (node.label.startsWith('token:')) return node.label.slice('token:'.length)
      node = node.parent
    }
    return null
  }

  /** screenBox is the bounding box of the token on the screen, or null for an unknown token. */
  screenBox(id: string): { x: number; y: number; w: number; h: number } | null {
    const view = this.views.get(id)
    const token = this.model?.scene?.tokens.find((t) => t.id === id)
    const sizePx = this.model?.scene?.grid.sizePx
    if (view === undefined || token === undefined || sizePx === undefined) return null
    const c = worldToScreen(this.camera, { x: view.x, y: view.y })
    const w = token.w * sizePx * this.camera.zoom
    const h = token.h * sizePx * this.camera.zoom
    return { x: c.x - w / 2, y: c.y - h / 2, w, h }
  }

  destroy(): void {
    this.destroyed = true
    this.dropViews()
    this.ghostView.destroy()
    for (const tex of this.textures.values()) tex?.destroy(true)
    this.textures.clear()
  }

  private layerOf(t: Token): Container {
    if (t.layer === 'map') return this.layers.map
    return t.layer === 'gm' ? this.layers.gm : this.layers.tokens
  }

  private dropViews(): void {
    for (const view of this.views.values()) view.destroy()
    this.views.clear()
  }

  private draw(m: TokenModel): void {
    const scene = m.scene
    if (scene === null) {
      this.ghostView.root.visible = false
      this.selection.clear()
      this.distanceLabel.root.visible = false
      return
    }
    const sizePx = scene.grid.sizePx
    for (const t of scene.tokens) {
      const view = this.views.get(t.id)
      if (view === undefined) continue
      const style = tokenStyle(t, m.players, m.isGM)
      const showName =
        style.showName === 'always' ||
        (style.showName === 'hover' && (m.hoverId === t.id || m.selectedId === t.id))
      view.draw(t, style, sizePx, this.camera, {
        texture: this.textureOf(t.image),
        showName,
        withBars: true,
        alpha: 1,
      })
    }

    const ghost = m.ghost
    this.ghostView.root.visible = ghost !== null
    if (ghost !== null) {
      this.ghostView.x = ghost.x
      this.ghostView.y = ghost.y
      this.ghostView.draw(
        ghost.token,
        tokenStyle(ghost.token, m.players, m.isGM),
        sizePx,
        this.camera,
        {
          texture: this.textureOf(ghost.token.image),
          showName: false,
          withBars: false,
          alpha: ghostAlpha,
        },
      )
    }

    this.selection.clear()
    const box = m.selectedId === null ? null : this.screenBox(m.selectedId)
    if (box !== null) {
      // The frame goes around the bars above the token too.
      const bars = scene.tokens.find((t) => t.id === m.selectedId)?.bars.length ?? 0
      const above = bars * (barHeight + barGap)
      dashedRect(
        this.selection,
        box.x - selectPad,
        box.y - selectPad - above,
        box.w + selectPad * 2,
        box.h + selectPad * 2 + above,
      )
      this.selection.stroke({ width: 1.5, color: this.colors.chalk })
    }

    const label = this.distanceLabel
    label.root.visible = m.distance !== null
    if (m.distance !== null) {
      label.set(m.distance.text)
      const view = this.app.screen
      const x = Math.min(m.distance.at.x + 16, view.width - label.width() - 4)
      const y = Math.min(m.distance.at.y + 16, view.height - label.height() - 4)
      label.root.position.set(Math.max(x, 4), Math.max(y, 4))
    }
  }

  private textureOf(image: AssetRef | null): Texture | null {
    return image === null ? null : (this.textures.get(image.assetId) ?? null)
  }

  private requestTexture(ref: AssetRef): void {
    if (this.textures.has(ref.assetId) || this.loading.has(ref.assetId)) return
    this.loading.add(ref.assetId)
    void this.loadTexture(ref)
  }

  private async loadTexture(ref: AssetRef): Promise<void> {
    let texture: Texture | null = null
    try {
      const res = await fetch(ref.url, { credentials: 'same-origin' })
      if (!res.ok) throw new Error(`HTTP ${String(res.status)}`)
      const bitmap = await createImageBitmap(await res.blob())
      texture = Texture.from(bitmap)
    } catch (err) {
      console.warn(`scene: cannot load the token image ${ref.assetId}`, err)
    }
    this.loading.delete(ref.assetId)
    if (this.destroyed) {
      texture?.destroy(true)
      return
    }
    this.textures.set(ref.assetId, texture)
    this.dirty = true
  }

  /** releaseTextures unloads the textures no token uses any more. */
  private releaseTextures(used: ReadonlySet<string>): void {
    for (const [assetId, texture] of this.textures) {
      if (used.has(assetId)) continue
      this.textures.delete(assetId)
      // The sprites that still hold it are destroyed with their views or rebuilt on the next draw.
      texture?.destroy(true)
    }
  }
}

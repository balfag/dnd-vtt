import type { PlayerInfo, Token, TokenBar, TokenFields } from '../../shared/protocol'

/** SizePreset is a size of SRD 5.2 in cells; `custom` takes the width and the height from the form. */
export type SizePreset = 'tiny' | 'medium' | 'large' | 'huge' | 'gargantuan' | 'custom'

/** sizePresets lists the presets in the order of the dialog, with their edge in cells. */
export const sizePresets: readonly { preset: Exclude<SizePreset, 'custom'>; cells: number }[] = [
  { preset: 'tiny', cells: 0.5 },
  { preset: 'medium', cells: 1 },
  { preset: 'large', cells: 2 },
  { preset: 'huge', cells: 3 },
  { preset: 'gargantuan', cells: 4 },
]

/** BarForm holds a bar the way the inputs do: numbers as text. */
export interface BarForm {
  key: string
  value: string
  max: string
  visibility: TokenBar['visibility']
}

export interface TokenForm {
  name: string
  nameVisibility: Token['nameVisibility']
  /** null: no image. */
  imageAssetId: string | null
  size: SizePreset
  /** Edge lengths in cells; read only when the size is custom. */
  width: string
  height: string
  rotation: string
  layer: Token['layer']
  hidden: boolean
  locked: boolean
  owners: string[]
  bars: BarForm[]
  conditions: string[]
}

/** TokenUpdate is the data of token.update. */
export type TokenUpdate = { tokenId: string } & TokenFields

/** The limits of docs/architecture/data-model.md, "Token". */
export const nameMax = 100
export const sizeCells = { min: 0.25, max: 20 }
export const maxBars = 5
export const barKeyPattern = /^[a-z0-9_]{1,16}$/
export const barValue = { min: -9999, max: 99999 }
export const barMax = { min: 1, max: 99999 }
export const rangeKey = 'tokens.error.range'
export const barKeyErrorKey = 'tokens.error.barKey'
export const barKeyTakenKey = 'tokens.error.barKeyTaken'

/** TokenErrors maps a field to the i18n key of its error: name, width, height, rotation, bars, bar.<i>.key|value|max. */
export type TokenErrors = Record<string, string>

function numberOf(text: string): number {
  const trimmed = text.trim()
  return trimmed === '' ? Number.NaN : Number(trimmed)
}

function textOf(n: number): string {
  return String(n)
}

function presetOf(w: number, h: number): SizePreset {
  if (w !== h) return 'custom'
  return sizePresets.find((p) => p.cells === w)?.preset ?? 'custom'
}

/**
 * ownerChoices are the players a token can belong to: not the GM (the GM needs no ownership)
 * and not the removed ones, who are not in the list at all.
 */
export function ownerChoices(players: readonly PlayerInfo[]): PlayerInfo[] {
  return players.filter((p) => p.role !== 'gm')
}

/** formFromToken starts the form from the token; owners that are not players of the campaign any more are dropped. */
export function formFromToken(token: Token, players: readonly PlayerInfo[]): TokenForm {
  const known = new Set(ownerChoices(players).map((p) => p.id))
  return {
    name: token.name,
    nameVisibility: token.nameVisibility,
    imageAssetId: token.image?.assetId ?? null,
    size: presetOf(token.w, token.h),
    width: textOf(token.w),
    height: textOf(token.h),
    rotation: textOf(token.rotation),
    layer: token.layer,
    hidden: token.hidden,
    locked: token.locked,
    owners: token.owners.filter((id) => known.has(id)),
    bars: token.bars.map((b) => ({
      key: b.key,
      value: textOf(b.value),
      max: textOf(b.max),
      visibility: b.visibility,
    })),
    conditions: [...token.conditions],
  }
}

/** sizeOf is the width and the height in cells the form asks for. */
export function sizeOf(form: TokenForm): { w: number; h: number } {
  const preset = sizePresets.find((p) => p.preset === form.size)
  if (preset !== undefined) return { w: preset.cells, h: preset.cells }
  return { w: numberOf(form.width), h: numberOf(form.height) }
}

/** newBar is a row for the "Add a bar" button: the first one is the hit points. */
export function newBar(existing: readonly BarForm[]): BarForm {
  return { key: existing.length === 0 ? 'hp' : '', value: '0', max: '1', visibility: 'all' }
}

function inRange(value: number, min: number, max: number, whole: boolean): boolean {
  return (
    Number.isFinite(value) && (!whole || Number.isInteger(value)) && value >= min && value <= max
  )
}

/** validateTokenForm returns the i18n key of the error of every invalid field. */
export function validateTokenForm(form: TokenForm): TokenErrors {
  const errors: TokenErrors = {}
  if (Array.from(form.name.trim()).length > nameMax) errors.name = rangeKey
  if (form.size === 'custom') {
    if (!inRange(numberOf(form.width), sizeCells.min, sizeCells.max, false)) errors.width = rangeKey
    if (!inRange(numberOf(form.height), sizeCells.min, sizeCells.max, false))
      errors.height = rangeKey
  }
  if (!Number.isFinite(numberOf(form.rotation))) errors.rotation = rangeKey
  if (form.bars.length > maxBars) errors.bars = rangeKey
  const seen = new Set<string>()
  form.bars.forEach((bar, i) => {
    if (!barKeyPattern.test(bar.key)) errors[`bar.${String(i)}.key`] = barKeyErrorKey
    else if (seen.has(bar.key)) errors[`bar.${String(i)}.key`] = barKeyTakenKey
    seen.add(bar.key)
    if (!inRange(numberOf(bar.value), barValue.min, barValue.max, true)) {
      errors[`bar.${String(i)}.value`] = rangeKey
    }
    if (!inRange(numberOf(bar.max), barMax.min, barMax.max, true)) {
      errors[`bar.${String(i)}.max`] = rangeKey
    }
  })
  return errors
}

/** limitsOf tells the allowed range of a field that has one, for the text of its error. */
export function limitsOf(field: string): { min: number; max: number } {
  if (field === 'name') return { min: 0, max: nameMax }
  if (field === 'width' || field === 'height') return sizeCells
  if (field.endsWith('.max')) return barMax
  if (field === 'bars') return { min: 0, max: maxBars }
  return barValue
}

function barsOf(form: TokenForm): TokenBar[] {
  return form.bars.map((b) => ({
    key: b.key,
    value: numberOf(b.value),
    max: numberOf(b.max),
    visibility: b.visibility,
  }))
}

function sameBars(a: readonly TokenBar[], b: readonly TokenBar[]): boolean {
  return (
    a.length === b.length &&
    a.every((x, i) => {
      const y = b[i]
      return (
        y !== undefined &&
        x.key === y.key &&
        x.value === y.value &&
        x.max === y.max &&
        x.visibility === y.visibility
      )
    })
  )
}

function sameSet(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x))
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i])
}

/**
 * buildTokenUpdate makes the token.update of the changed fields only, or null when nothing
 * changed. owners, bars and conditions go whole. The form must be valid.
 */
export function buildTokenUpdate(original: Token, form: TokenForm): TokenUpdate | null {
  const update: TokenUpdate = { tokenId: original.id }
  let changed = false

  const name = form.name.trim()
  if (name !== original.name) {
    update.name = name
    changed = true
  }
  if (form.nameVisibility !== original.nameVisibility) {
    update.nameVisibility = form.nameVisibility
    changed = true
  }
  if (form.imageAssetId !== (original.image?.assetId ?? null)) {
    update.imageAssetId = form.imageAssetId
    changed = true
  }
  const size = sizeOf(form)
  if (size.w !== original.w) {
    update.w = size.w
    changed = true
  }
  if (size.h !== original.h) {
    update.h = size.h
    changed = true
  }
  const rotation = numberOf(form.rotation)
  if (rotation !== original.rotation) {
    update.rotation = rotation
    changed = true
  }
  if (form.layer !== original.layer) {
    update.layer = form.layer
    changed = true
  }
  if (form.hidden !== original.hidden) {
    update.hidden = form.hidden
    changed = true
  }
  if (form.locked !== original.locked) {
    update.locked = form.locked
    changed = true
  }
  if (!sameList(form.owners, original.owners)) {
    update.owners = [...form.owners]
    changed = true
  }
  const bars = barsOf(form)
  if (!sameBars(bars, original.bars)) {
    update.bars = bars
    changed = true
  }
  if (!sameSet(form.conditions, original.conditions)) {
    update.conditions = [...form.conditions]
    changed = true
  }
  return changed ? update : null
}

/** tokenNameFromFile makes the name of a new token of the name of its image file: no extension, 100 characters at most. */
export function tokenNameFromFile(originalName: string): string {
  const dot = originalName.lastIndexOf('.')
  const base = dot > 0 ? originalName.slice(0, dot) : originalName
  return Array.from(base.trim()).slice(0, nameMax).join('')
}

/** buildTokenCreate is the token.create of a new token of one cell on the tokens layer. */
export function buildTokenCreate(
  sceneId: string,
  at: { x: number; y: number },
  image: { assetId: string; originalName: string } | null,
): { sceneId: string } & TokenFields {
  const create: { sceneId: string } & TokenFields = {
    sceneId,
    layer: 'tokens',
    x: at.x,
    y: at.y,
    w: 1,
    h: 1,
    name: image === null ? '' : tokenNameFromFile(image.originalName),
  }
  if (image !== null) create.imageAssetId = image.assetId
  return create
}

/** zOrder is the z that puts the token in front of the others of its layer, or behind them. */
export function zOrder(token: Token, tokens: readonly Token[], where: 'front' | 'back'): number {
  const zs = tokens.filter((t) => t.layer === token.layer).map((t) => t.z)
  return where === 'front' ? Math.max(token.z, ...zs) + 1 : Math.min(token.z, ...zs) - 1
}

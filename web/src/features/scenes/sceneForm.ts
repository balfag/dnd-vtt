import type { Grid, SceneInfo, SceneUpdate } from '../../shared/protocol'

/** SceneField is a field of the form that has a limit. */
export type SceneField = 'name' | 'width' | 'height' | 'sizePx' | 'unitFt' | 'offsetX' | 'offsetY'

/** GridForm holds the grid the way the inputs do: numbers as text. */
export interface GridForm {
  type: Grid['type']
  sizePx: string
  unitFt: string
  offsetX: string
  offsetY: string
  color: string
  visible: boolean
}

export interface SceneForm {
  name: string
  width: string
  height: string
  /** null: no background. */
  backgroundAssetId: string | null
  grid: GridForm
}

/** The limits of docs/architecture/data-model.md, "Scene". */
/** nameLimits counts characters of the trimmed name. */
export const nameLimits = { min: 1, max: 200 }
const sceneSize = { min: 100, max: 20000 }
const cellSize = { min: 10, max: 500 }
const unitFeet = { min: 1, max: 100 }

export const rangeKey = 'scenes.error.range'

export function formFromScene(scene: SceneInfo): SceneForm {
  const g = scene.grid
  return {
    name: scene.name,
    width: String(scene.width),
    height: String(scene.height),
    backgroundAssetId: scene.background?.assetId ?? null,
    grid: {
      type: g.type,
      sizePx: String(g.sizePx),
      unitFt: String(g.unitFt),
      offsetX: String(g.offsetX),
      offsetY: String(g.offsetY),
      color: g.color,
      visible: g.visible,
    },
  }
}

function numberOf(text: string): number {
  const trimmed = text.trim()
  return trimmed === '' ? Number.NaN : Number(trimmed)
}

/** fieldLimits tells the allowed range of a field; the offsets depend on the cell size. */
export function fieldLimits(field: SceneField, form: SceneForm): { min: number; max: number } {
  switch (field) {
    case 'name':
      return nameLimits
    case 'width':
    case 'height':
      return sceneSize
    case 'sizePx':
      return cellSize
    case 'unitFt':
      return unitFeet
    case 'offsetX':
    case 'offsetY': {
      const size = numberOf(form.grid.sizePx)
      return { min: 0, max: Number.isFinite(size) ? size : cellSize.max }
    }
  }
}

function valueOf(field: SceneField, form: SceneForm): number {
  switch (field) {
    case 'name':
      return Array.from(form.name.trim()).length
    case 'width':
      return numberOf(form.width)
    case 'height':
      return numberOf(form.height)
    case 'sizePx':
    case 'unitFt':
    case 'offsetX':
    case 'offsetY':
      return numberOf(form.grid[field])
  }
}

/** Offsets may be fractions; every other number is whole. */
function needsInteger(field: SceneField): boolean {
  return field !== 'offsetX' && field !== 'offsetY' && field !== 'name'
}

const fields: SceneField[] = ['name', 'width', 'height', 'sizePx', 'unitFt', 'offsetX', 'offsetY']

/** validateSceneForm returns the i18n key of the error of every invalid field. */
export function validateSceneForm(form: SceneForm): Partial<Record<SceneField, string>> {
  const errors: Partial<Record<SceneField, string>> = {}
  for (const field of fields) {
    const value = valueOf(field, form)
    const { min, max } = fieldLimits(field, form)
    const whole = !needsInteger(field) || Number.isInteger(value)
    if (!Number.isFinite(value) || !whole || value < min || value > max) {
      errors[field] = rangeKey
    }
  }
  return errors
}

function gridOf(g: GridForm): Grid {
  return {
    type: g.type,
    sizePx: numberOf(g.sizePx),
    unitFt: numberOf(g.unitFt),
    offsetX: numberOf(g.offsetX),
    offsetY: numberOf(g.offsetY),
    color: g.color,
    visible: g.visible,
  }
}

function sameGrid(a: Grid, b: Grid): boolean {
  return (
    a.type === b.type &&
    a.sizePx === b.sizePx &&
    a.unitFt === b.unitFt &&
    a.offsetX === b.offsetX &&
    a.offsetY === b.offsetY &&
    a.color === b.color &&
    a.visible === b.visible
  )
}

/**
 * buildSceneUpdate makes the scene.update of the changed fields only, or null
 * when nothing changed. The form must be valid.
 */
export function buildSceneUpdate(original: SceneInfo, form: SceneForm): SceneUpdate | null {
  const update: SceneUpdate = { sceneId: original.id }
  let changed = false

  const name = form.name.trim()
  if (name !== original.name) {
    update.name = name
    changed = true
  }
  const width = numberOf(form.width)
  if (width !== original.width) {
    update.width = width
    changed = true
  }
  const height = numberOf(form.height)
  if (height !== original.height) {
    update.height = height
    changed = true
  }
  const grid = gridOf(form.grid)
  if (!sameGrid(grid, original.grid)) {
    update.grid = grid
    changed = true
  }
  if (form.backgroundAssetId !== (original.background?.assetId ?? null)) {
    update.backgroundAssetId = form.backgroundAssetId
    changed = true
  }
  return changed ? update : null
}

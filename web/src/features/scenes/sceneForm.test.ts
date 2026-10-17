import { describe, expect, it } from 'vitest'
import type { SceneInfo } from '../../shared/protocol'
import {
  buildSceneUpdate,
  fieldLimits,
  formFromScene,
  validateSceneForm,
  type SceneForm,
} from './sceneForm'

const scene: SceneInfo = {
  id: 's1',
  name: 'Tavern',
  width: 2800,
  height: 2100,
  grid: {
    type: 'square',
    sizePx: 70,
    unitFt: 5,
    offsetX: 0,
    offsetY: 0,
    color: '#9aa8b3',
    visible: true,
  },
  background: { assetId: 'a1', url: '/assets/a1' },
  fog: { enabled: false, ops: [] },
}

function form(patch: Partial<Omit<SceneForm, 'grid'>> = {}, grid: Partial<SceneForm['grid']> = {}) {
  const base = formFromScene(scene)
  return { ...base, ...patch, grid: { ...base.grid, ...grid } }
}

describe('validateSceneForm', () => {
  it('accepts the form of a scene', () => {
    expect(validateSceneForm(form())).toEqual({})
  })

  it('checks the name by characters after trimming', () => {
    expect(validateSceneForm(form({ name: '' })).name).toBeDefined()
    expect(validateSceneForm(form({ name: '   ' })).name).toBeDefined()
    expect(validateSceneForm(form({ name: 'я'.repeat(200) })).name).toBeUndefined()
    expect(validateSceneForm(form({ name: 'я'.repeat(201) })).name).toBeDefined()
    expect(validateSceneForm(form({ name: '🎲'.repeat(200) })).name).toBeUndefined()
  })

  it.each(['width', 'height'] as const)('checks %s from 100 to 20000', (field) => {
    expect(validateSceneForm(form({ [field]: '99' }))[field]).toBe('scenes.error.range')
    expect(validateSceneForm(form({ [field]: '100' }))[field]).toBeUndefined()
    expect(validateSceneForm(form({ [field]: '20000' }))[field]).toBeUndefined()
    expect(validateSceneForm(form({ [field]: '20001' }))[field]).toBe('scenes.error.range')
    expect(validateSceneForm(form({ [field]: '100.5' }))[field]).toBeDefined()
    expect(validateSceneForm(form({ [field]: '' }))[field]).toBeDefined()
    expect(validateSceneForm(form({ [field]: 'abc' }))[field]).toBeDefined()
  })

  it('checks the cell size from 10 to 500', () => {
    expect(validateSceneForm(form({}, { sizePx: '9' })).sizePx).toBeDefined()
    expect(validateSceneForm(form({}, { sizePx: '10', offsetX: '0' })).sizePx).toBeUndefined()
    expect(validateSceneForm(form({}, { sizePx: '500' })).sizePx).toBeUndefined()
    expect(validateSceneForm(form({}, { sizePx: '501' })).sizePx).toBeDefined()
  })

  it('checks the feet per cell from 1 to 100', () => {
    expect(validateSceneForm(form({}, { unitFt: '0' })).unitFt).toBeDefined()
    expect(validateSceneForm(form({}, { unitFt: '1' })).unitFt).toBeUndefined()
    expect(validateSceneForm(form({}, { unitFt: '100' })).unitFt).toBeUndefined()
    expect(validateSceneForm(form({}, { unitFt: '101' })).unitFt).toBeDefined()
  })

  it('keeps the offsets within the cell and allows fractions', () => {
    const f = form({}, { sizePx: '50', offsetX: '50', offsetY: '12.5' })
    expect(validateSceneForm(f)).toEqual({})
    expect(validateSceneForm({ ...f, grid: { ...f.grid, offsetX: '50.5' } }).offsetX).toBeDefined()
    expect(validateSceneForm({ ...f, grid: { ...f.grid, offsetY: '-1' } }).offsetY).toBeDefined()
    expect(fieldLimits('offsetX', f)).toEqual({ min: 0, max: 50 })
  })

  it('reports every bad field at once', () => {
    const errors = validateSceneForm(form({ name: '', width: '1' }, { unitFt: '0' }))
    expect(Object.keys(errors).sort()).toEqual(['name', 'unitFt', 'width'])
  })
})

describe('buildSceneUpdate', () => {
  it('is null when nothing changed', () => {
    expect(buildSceneUpdate(scene, form())).toBeNull()
  })

  it('sends only the changed fields', () => {
    expect(buildSceneUpdate(scene, form({ name: ' Crypt ' }))).toEqual({
      sceneId: 's1',
      name: 'Crypt',
    })
    expect(buildSceneUpdate(scene, form({ width: '3000' }))).toEqual({
      sceneId: 's1',
      width: 3000,
    })
    expect(buildSceneUpdate(scene, form({ height: '2000' }))).toEqual({
      sceneId: 's1',
      height: 2000,
    })
  })

  it('sends null for a removed background and the id for a new one', () => {
    expect(buildSceneUpdate(scene, form({ backgroundAssetId: null }))).toEqual({
      sceneId: 's1',
      backgroundAssetId: null,
    })
    expect(buildSceneUpdate(scene, form({ backgroundAssetId: 'a2' }))).toEqual({
      sceneId: 's1',
      backgroundAssetId: 'a2',
    })
  })

  it('does not touch a background that was not there and is not set', () => {
    expect(
      buildSceneUpdate({ ...scene, background: null }, form({ backgroundAssetId: null })),
    ).toBeNull()
  })

  it('sends the whole grid when one grid field changed', () => {
    expect(buildSceneUpdate(scene, form({}, { type: 'hexFlat' }))).toEqual({
      sceneId: 's1',
      grid: { ...scene.grid, type: 'hexFlat' },
    })
    expect(buildSceneUpdate(scene, form({}, { visible: false, offsetY: '7.5' }))).toEqual({
      sceneId: 's1',
      grid: { ...scene.grid, visible: false, offsetY: 7.5 },
    })
  })

  it('treats a number written with spaces as unchanged', () => {
    expect(buildSceneUpdate(scene, form({ width: ' 2800 ' }, { sizePx: '70 ' }))).toBeNull()
  })
})

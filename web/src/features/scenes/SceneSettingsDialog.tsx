import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Asset } from '../../shared/api/assets'
import { MessageType, type Grid, type SceneInfo } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'
import { BackgroundPicker } from './BackgroundPicker'
import {
  buildSceneUpdate,
  fieldLimits,
  formFromScene,
  validateSceneForm,
  type GridForm,
  type SceneField,
  type SceneForm,
} from './sceneForm'
import './scenes.css'

interface SceneSettingsDialogProps {
  open: boolean
  /** original is the scene as it was when the dialog was opened. */
  original: SceneInfo
  campaignId: string
  command: (type: string, data?: unknown) => Promise<unknown>
  onClose: () => void
}

const gridTypes: { type: Grid['type']; label: string }[] = [
  { type: 'square', label: 'scenes.grid.square' },
  { type: 'hexPointy', label: 'scenes.grid.hexPointy' },
  { type: 'hexFlat', label: 'scenes.grid.hexFlat' },
]

/**
 * SceneSettingsDialog edits the name, the size, the background and the grid of a scene.
 * Its form starts from `original` when the component mounts: remount it (a new key) per opening.
 */
export function SceneSettingsDialog({
  open,
  original,
  campaignId,
  command,
  onClose,
}: SceneSettingsDialogProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<SceneForm>(() => formFromScene(original))
  const [assets, setAssets] = useState<Asset[]>([])
  const [shown, setShown] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const formEl = useRef<HTMLFormElement>(null)
  const id = useId()

  const errors = validateSceneForm(form)
  const chosen = assets.find((a) => a.id === form.backgroundAssetId)

  function setGrid(patch: Partial<GridForm>) {
    setForm((f) => ({ ...f, grid: { ...f.grid, ...patch } }))
  }

  function errorText(field: SceneField): string | undefined {
    const key = errors[field]
    if (!shown || key === undefined) return undefined
    return t(key, fieldLimits(field, form))
  }

  async function save() {
    if (busy) return
    setShown(true)
    if (Object.keys(errors).length > 0) {
      // The errors are drawn after this render: focus the first one then.
      requestAnimationFrame(() => {
        formEl.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
      })
      return
    }
    const update = buildSceneUpdate(original, form)
    if (update === null) {
      onClose()
      return
    }
    setBusy(true)
    setFailure(null)
    try {
      await command(MessageType.SceneUpdate, update)
      onClose()
    } catch (err) {
      setFailure(commandErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open={open}
      title={t('scenes.settings')}
      closeLabel={t('ui.dialog.cancel')}
      onClose={onClose}
      actions={
        <Button
          fullWidth={false}
          disabled={busy}
          onClick={() => {
            void save()
          }}
        >
          {t('scenes.save')}
        </Button>
      }
    >
      <form
        ref={formEl}
        className="scene-form"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void save()
        }}
      >
        <Field
          id={`${id}-name`}
          label={t('scenes.field.name')}
          value={form.name}
          error={errorText('name')}
          autoComplete="off"
          onChange={(name) => {
            setForm((f) => ({ ...f, name }))
          }}
        />

        <fieldset className="scene-form__group">
          <legend>{t('scenes.bg.label')}</legend>
          <BackgroundPicker
            campaignId={campaignId}
            value={form.backgroundAssetId}
            onChange={(backgroundAssetId) => {
              setForm((f) => ({ ...f, backgroundAssetId }))
            }}
            onAssets={setAssets}
          />
        </fieldset>

        <div className="scene-form__row">
          <Field
            id={`${id}-width`}
            label={t('scenes.field.width')}
            value={form.width}
            error={errorText('width')}
            autoComplete="off"
            onChange={(width) => {
              setForm((f) => ({ ...f, width }))
            }}
          />
          <Field
            id={`${id}-height`}
            label={t('scenes.field.height')}
            value={form.height}
            error={errorText('height')}
            autoComplete="off"
            onChange={(height) => {
              setForm((f) => ({ ...f, height }))
            }}
          />
        </div>
        <div>
          <Button
            variant="secondary"
            fullWidth={false}
            disabled={chosen === undefined}
            onClick={() => {
              if (chosen === undefined) return
              setForm((f) => ({ ...f, width: String(chosen.width), height: String(chosen.height) }))
            }}
          >
            {t('scenes.size.fromBg')}
          </Button>
        </div>

        <fieldset className="scene-form__group">
          <legend>{t('scenes.grid.label')}</legend>
          <div className="scene-form__radios" role="radiogroup" aria-label={t('scenes.grid.type')}>
            {gridTypes.map((g) => (
              <label key={g.type} className="scene-form__radio">
                <input
                  type="radio"
                  name={`${id}-grid-type`}
                  checked={form.grid.type === g.type}
                  onChange={() => {
                    setGrid({ type: g.type })
                  }}
                />
                {t(g.label)}
              </label>
            ))}
          </div>
          <div className="scene-form__row">
            <Field
              id={`${id}-size`}
              label={t('scenes.grid.size')}
              value={form.grid.sizePx}
              error={errorText('sizePx')}
              autoComplete="off"
              onChange={(sizePx) => {
                setGrid({ sizePx })
              }}
            />
            <Field
              id={`${id}-unit`}
              label={t('scenes.grid.unitFt')}
              value={form.grid.unitFt}
              error={errorText('unitFt')}
              autoComplete="off"
              onChange={(unitFt) => {
                setGrid({ unitFt })
              }}
            />
            <Field
              id={`${id}-ox`}
              label={t('scenes.grid.offsetX')}
              value={form.grid.offsetX}
              error={errorText('offsetX')}
              autoComplete="off"
              onChange={(offsetX) => {
                setGrid({ offsetX })
              }}
            />
            <Field
              id={`${id}-oy`}
              label={t('scenes.grid.offsetY')}
              value={form.grid.offsetY}
              error={errorText('offsetY')}
              autoComplete="off"
              onChange={(offsetY) => {
                setGrid({ offsetY })
              }}
            />
          </div>
          <div className="scene-form__row scene-form__row--center">
            <label className="scene-form__color">
              {t('scenes.grid.color')}
              <input
                type="color"
                value={form.grid.color}
                onChange={(e) => {
                  setGrid({ color: e.target.value })
                }}
              />
            </label>
            <label className="scene-form__radio">
              <input
                type="checkbox"
                checked={form.grid.visible}
                onChange={(e) => {
                  setGrid({ visible: e.target.checked })
                }}
              />
              {t('scenes.grid.visible')}
            </label>
          </div>
        </fieldset>

        {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
        {/* Enter in a field saves, as the button does. */}
        <button type="submit" hidden />
      </form>
    </Dialog>
  )
}

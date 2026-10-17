import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageType, type SceneView } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import type { FogShapeKind } from '../table/canvas/fogShapes'
import './fog.css'

/** FogToolState is the fog tool the GM has chosen and how it draws. */
export interface FogToolState {
  mode: 'reveal' | 'hide'
  shape: FogShapeKind
  /** The brush radius in cells. */
  radiusCells: number
}

export const MAX_FOG_OPS = 500
/** The counter appears above this many operations. */
const showOpsAbove = 400
export const MIN_BRUSH_CELLS = 0.5
export const MAX_BRUSH_CELLS = 5
const shapes: FogShapeKind[] = ['rect', 'polygon', 'brush']

interface FogToolbarProps {
  scene: SceneView
  tool: FogToolState
  onChange: (next: FogToolState) => void
  command: (type: string, data?: unknown) => Promise<unknown>
  announce: (text: string, visible: boolean) => void
}

/** FogToolbar is the options of the active fog tool, next to the tool strip. */
export function FogToolbar({ scene, tool, onChange, command, announce }: FogToolbarProps) {
  const { t } = useTranslation()
  const [confirming, setConfirming] = useState(false)
  const count = scene.fog.ops.length

  function run(type: string, data: unknown) {
    command(type, data).catch((err: unknown) => {
      announce(t(commandErrorKey(err)), true)
    })
  }

  return (
    <div className="fogbar" role="group" aria-label={t('fog.title')}>
      <h2 className="fogbar__title">{t(`fog.tool.${tool.mode}`)}</h2>
      <fieldset className="fogbar__shapes">
        <legend className="fogbar__legend">{t('fog.shape.label')}</legend>
        {shapes.map((kind) => (
          <label key={kind} className="fogbar__shape">
            <input
              type="radio"
              name="fog-shape"
              checked={tool.shape === kind}
              onChange={() => {
                onChange({ ...tool, shape: kind })
              }}
            />
            <span>{t(`fog.shape.${kind}`)}</span>
          </label>
        ))}
      </fieldset>
      {tool.shape === 'brush' && (
        <label className="fogbar__row">
          <span>{t('fog.brush.size')}</span>
          <span className="fogbar__range">
            <input
              type="range"
              min={MIN_BRUSH_CELLS}
              max={MAX_BRUSH_CELLS}
              step={0.5}
              value={tool.radiusCells}
              onChange={(e) => {
                onChange({ ...tool, radiusCells: Number(e.target.value) })
              }}
            />
            <output>{tool.radiusCells}</output>
          </span>
        </label>
      )}
      {tool.shape === 'polygon' && <p className="fogbar__hint">{t('fog.polygonHint')}</p>}
      <label className="fogbar__row fogbar__row--check">
        <input
          type="checkbox"
          role="switch"
          checked={scene.fog.enabled}
          onChange={(e) => {
            run(MessageType.FogSet, { sceneId: scene.id, enabled: e.target.checked })
          }}
        />
        <span>{t('fog.enabled')}</span>
      </label>
      <button
        type="button"
        className="fogbar__reset"
        onClick={() => {
          setConfirming(true)
        }}
      >
        {t('fog.reset')}
      </button>
      {count > showOpsAbove && (
        <div className="fogbar__ops">
          <p className="fogbar__count" data-full={count >= MAX_FOG_OPS ? '' : undefined}>
            {t('fog.ops', { count })}
          </p>
          {count >= MAX_FOG_OPS && <p className="fogbar__full">{t('fog.opsFull')}</p>}
        </div>
      )}
      <Dialog
        open={confirming}
        title={t('fog.reset')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirming(false)
        }}
        actions={
          <Button
            variant="danger"
            fullWidth={false}
            onClick={() => {
              setConfirming(false)
              run(MessageType.FogReset, { sceneId: scene.id })
            }}
          >
            {t('fog.reset')}
          </Button>
        }
      >
        <p>{t('fog.resetConfirm')}</p>
      </Dialog>
    </div>
  )
}

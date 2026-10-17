import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageType, type SceneInfo } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { gameStore, useGame, viewScene } from '../game/store'
import { nameLimits, rangeKey } from './sceneForm'
import { SceneRow } from './SceneRow'
import './scenes.css'

interface ScenesPanelProps {
  command: (type: string, data?: unknown) => Promise<unknown>
}

/** createdId reads the id of the new scene from the ack of scene.create. */
function createdId(d: unknown): string | null {
  if (typeof d === 'object' && d !== null && 'sceneId' in d && typeof d.sceneId === 'string') {
    return d.sceneId
  }
  return null
}

/** ScenesPanel is the GM's list of scenes: look at one, show it to the players, create, order, edit. */
export function ScenesPanel({ command }: ScenesPanelProps) {
  const { t } = useTranslation()
  const list = useGame((s) => s.game?.scenes)
  const activeId = useGame((s) => s.game?.activeSceneId ?? null)
  const viewedId = useGame((s) => s.game?.viewSceneId ?? null)
  const campaignId = useGame((s) => s.game?.campaign.id ?? '')
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [draft, setDraft] = useState('')
  const [draftError, setDraftError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const draftInput = useRef<HTMLInputElement>(null)
  const newButton = useRef<HTMLButtonElement>(null)
  const restoreFocus = useRef(false)

  const scenes = [...(list ?? [])].sort((a, b) => a.sort - b.sort)
  const isEmpty = scenes.length === 0

  useEffect(() => {
    if (creating) {
      draftInput.current?.focus()
    } else if (restoreFocus.current) {
      restoreFocus.current = false
      newButton.current?.focus()
    }
  }, [creating])

  function fail(err: unknown) {
    setError(commandErrorKey(err))
  }

  function view(sceneId: string) {
    if (sceneId === viewedId) return
    setError(null)
    viewScene(command, sceneId).catch(fail)
  }

  function show(sceneId: string) {
    setError(null)
    command(MessageType.SceneActivate, { sceneId }).catch(fail)
  }

  function move(sceneId: string, step: -1 | 1) {
    const ids = scenes.map((s) => s.id)
    const from = ids.indexOf(sceneId)
    const to = from + step
    if (from < 0 || to < 0 || to >= ids.length) return
    ids.splice(to, 0, ...ids.splice(from, 1))
    setError(null)
    command(MessageType.SceneReorder, { sceneIds: ids }).catch(fail)
  }

  async function rename(sceneId: string, name: string) {
    setError(null)
    await command(MessageType.SceneUpdate, { sceneId, name })
  }

  /** openSettings looks at the scene, so the store holds it whole, and returns it. */
  async function openSettings(sceneId: string): Promise<SceneInfo | null> {
    setError(null)
    try {
      if (gameStore.getState().game?.viewSceneId !== sceneId) await viewScene(command, sceneId)
    } catch (err) {
      fail(err)
      return null
    }
    const scene = gameStore.getState().game?.scene
    return scene?.id === sceneId ? scene : null
  }

  async function remove(sceneId: string) {
    setError(null)
    try {
      await command(MessageType.SceneDelete, { sceneId })
    } catch (err) {
      fail(err)
      throw err
    }
  }

  function openCreate() {
    setError(null)
    setDraft('')
    setDraftError(null)
    setCreating(true)
  }

  function closeCreate(restore: boolean) {
    restoreFocus.current = restore
    setCreating(false)
    setDraftError(null)
  }

  async function create() {
    const name = draft.trim()
    const runes = Array.from(name).length
    if (runes < nameLimits.min || runes > nameLimits.max) {
      setDraftError(rangeKey)
      return
    }
    setBusy(true)
    try {
      const sceneId = createdId(await command(MessageType.SceneCreate, { name }))
      closeCreate(true)
      if (sceneId !== null) await viewScene(command, sceneId)
    } catch (err) {
      setDraftError(commandErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  const newField = (
    <div className="scene-row__edit">
      <input
        ref={draftInput}
        className="field__input"
        aria-label={t('scenes.new')}
        aria-invalid={draftError !== null}
        placeholder={t('scenes.newPlaceholder')}
        value={draft}
        disabled={busy}
        onChange={(e) => {
          setDraft(e.target.value)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            void create()
          } else if (e.key === 'Escape') {
            e.preventDefault()
            closeCreate(true)
          }
        }}
        onBlur={() => {
          if (!busy) closeCreate(false)
        }}
      />
      {draftError !== null && (
        <p className="field__text field__text--error">{t(draftError, nameLimits)}</p>
      )}
    </div>
  )

  return (
    <div className="scenes">
      {isEmpty && !creating && <p className="scenes__empty">{t('scenes.empty')}</p>}
      {(!isEmpty || creating) && (
        <ul className="scenes__list" aria-label={t('scenes.tab')}>
          {scenes.map((scene, i) => (
            <SceneRow
              key={scene.id}
              scene={scene}
              campaignId={campaignId}
              isFirst={i === 0}
              isLast={i === scenes.length - 1}
              isActive={scene.id === activeId}
              isViewed={scene.id === viewedId}
              command={command}
              onView={() => {
                view(scene.id)
              }}
              onShow={() => {
                show(scene.id)
              }}
              onMove={(step) => {
                move(scene.id, step)
              }}
              onRename={(name) => rename(scene.id, name)}
              onSettings={() => openSettings(scene.id)}
              onDelete={() => remove(scene.id)}
              onFocusFallback={() => newButton.current?.focus()}
            />
          ))}
          {creating && <li className="scene-row scene-row--new">{newField}</li>}
        </ul>
      )}

      <div className="scenes__foot">
        {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
        {!creating && (
          <Button ref={newButton} variant="secondary" fullWidth={false} onClick={openCreate}>
            {t('scenes.new')}
          </Button>
        )}
      </div>
    </div>
  )
}

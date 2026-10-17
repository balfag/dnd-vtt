import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SceneInfo, SceneListItem } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { nameLimits, rangeKey } from './sceneForm'
import { SceneMenu, type MenuAction } from './SceneMenu'
import { SceneSettingsDialog } from './SceneSettingsDialog'

interface SceneRowProps {
  scene: SceneListItem
  campaignId: string
  isFirst: boolean
  isLast: boolean
  /** isActive: players see this scene. */
  isActive: boolean
  /** isViewed: this scene is on the GM's canvas. */
  isViewed: boolean
  command: (type: string, data?: unknown) => Promise<unknown>
  onView: () => void
  onShow: () => void
  onMove: (step: -1 | 1) => void
  /** onRename resolves when the server took the name. */
  onRename: (name: string) => Promise<void>
  /** onSettings makes the scene the viewed one and gives it whole, or null when it failed. */
  onSettings: () => Promise<SceneInfo | null>
  /** onDelete resolves when the scene is deleted. */
  onDelete: () => Promise<void>
  /** onFocusFallback is where the focus goes when the row is gone. */
  onFocusFallback: () => void
}

/** SceneRow is one scene of the list: the name, the state, the quick action and the menu. */
export function SceneRow({
  scene,
  campaignId,
  isFirst,
  isLast,
  isActive,
  isViewed,
  command,
  onView,
  onShow,
  onMove,
  onRename,
  onSettings,
  onDelete,
  onFocusFallback,
}: SceneRowProps) {
  const { t } = useTranslation()
  const [renaming, setRenaming] = useState(false)
  const [draft, setDraft] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [settings, setSettings] = useState<{
    scene: SceneInfo
    open: boolean
    nonce: number
  } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const more = useRef<HTMLButtonElement | null>(null)
  // The focus goes back to "⋯" when the rename ends by the keyboard.
  const restoreFocus = useRef(false)

  useEffect(() => {
    if (renaming) {
      input.current?.focus()
      input.current?.select()
    } else if (restoreFocus.current) {
      restoreFocus.current = false
      more.current?.focus()
    }
  }, [renaming])

  function endRename(restore: boolean) {
    restoreFocus.current = restore
    setRenaming(false)
    setFieldError(null)
  }

  async function submitRename() {
    const name = draft.trim()
    const runes = Array.from(name).length
    if (runes < nameLimits.min || runes > nameLimits.max) {
      setFieldError(rangeKey)
      return
    }
    if (name === scene.name) {
      endRename(true)
      return
    }
    setBusy(true)
    try {
      await onRename(name)
      endRename(true)
    } catch (err) {
      setFieldError(commandErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  function act(action: MenuAction) {
    switch (action) {
      case 'rename':
        setDraft(scene.name)
        setFieldError(null)
        setRenaming(true)
        break
      case 'moveUp':
        onMove(-1)
        more.current?.focus()
        break
      case 'moveDown':
        onMove(1)
        more.current?.focus()
        break
      case 'settings':
        void onSettings().then((whole) => {
          if (whole === null) return
          setSettings((prev) => ({ scene: whole, open: true, nonce: (prev?.nonce ?? 0) + 1 }))
        })
        break
      case 'delete':
        setConfirmDelete(true)
        break
    }
  }

  async function remove() {
    setConfirmDelete(false)
    try {
      await onDelete()
      onFocusFallback()
    } catch {
      // The panel already shows the reason.
      more.current?.focus()
    }
  }

  const className = [
    'scene-row',
    isViewed ? 'scene-row--viewed' : '',
    isActive ? 'scene-row--live' : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <li className={className} aria-current={isViewed ? 'true' : undefined}>
      {renaming ? (
        <div className="scene-row__edit">
          <input
            ref={input}
            className="field__input"
            aria-label={t('scenes.rename')}
            aria-invalid={fieldError !== null}
            value={draft}
            disabled={busy}
            onChange={(e) => {
              setDraft(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void submitRename()
              } else if (e.key === 'Escape') {
                e.preventDefault()
                endRename(true)
              }
            }}
            onBlur={() => {
              if (!busy) endRename(false)
            }}
          />
          {fieldError !== null && (
            <p className="field__text field__text--error">{t(fieldError, nameLimits)}</p>
          )}
        </div>
      ) : (
        <button type="button" className="scene-row__main" onClick={onView}>
          <span className="scene-row__name">{scene.name}</span>
          {isActive && <span className="scene-row__live">{t('scenes.live')}</span>}
        </button>
      )}
      {!renaming && !isActive && (
        <span className="scene-row__show">
          <Button variant="secondary" fullWidth={false} onClick={onShow}>
            {t('scenes.showToPlayers')}
          </Button>
        </span>
      )}
      {!renaming && (
        <SceneMenu
          sceneName={scene.name}
          canMoveUp={!isFirst}
          canMoveDown={!isLast}
          isActive={isActive}
          onAction={act}
          buttonRef={(el) => {
            more.current = el
          }}
        />
      )}

      {settings !== null && (
        <SceneSettingsDialog
          key={settings.nonce}
          open={settings.open}
          original={settings.scene}
          campaignId={campaignId}
          command={command}
          onClose={() => {
            setSettings((prev) => (prev === null ? null : { ...prev, open: false }))
            more.current?.focus()
          }}
        />
      )}
      <Dialog
        open={confirmDelete}
        title={t('scenes.delete')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirmDelete(false)
          more.current?.focus()
        }}
        actions={
          <Button
            variant="danger"
            fullWidth={false}
            onClick={() => {
              void remove()
            }}
          >
            {t('scenes.delete')}
          </Button>
        }
      >
        <p>{t('scenes.deleteConfirm', { name: scene.name })}</p>
      </Dialog>
    </li>
  )
}

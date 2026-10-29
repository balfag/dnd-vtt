import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorKey } from '../../../shared/api/errors'
import type { PageRef } from '../../../shared/api/wiki'
import { Dialog } from '../../../ui/Dialog'
import { AssetPicker } from '../../assets/AssetPicker'
import type { SaveState } from './autosave'
import { validIcon } from './icon'

interface PageHeaderProps {
  campaignId: string
  path: PageRef[]
  title: string
  icon: string
  coverAssetId: string | null
  saveState: SaveState
  /** saveError is what stopped the last save; its text is the hint of "Not saved". */
  saveError: unknown
  onTitle: (title: string) => void
  onIcon: (icon: string) => void
  onCover: (assetId: string | null) => void
  onOpenPath: (id: string) => void
  onRetry: () => void
}

const maxTitleLength = 200

/** PageHeader is the top of a page: cover, icon, path of parents, title and the state of saving. */
export function PageHeader(p: PageHeaderProps) {
  const { t } = useTranslation()
  const [editingIcon, setEditingIcon] = useState(false)
  const [iconDraft, setIconDraft] = useState(p.icon)
  const [pickingCover, setPickingCover] = useState(false)
  const iconInput = useRef<HTMLInputElement>(null)
  const iconButton = useRef<HTMLButtonElement>(null)
  const iconOk = validIcon(iconDraft)

  useEffect(() => {
    if (editingIcon) iconInput.current?.focus()
  }, [editingIcon])

  function endIcon(commit: boolean) {
    if (commit && iconOk && iconDraft !== p.icon) p.onIcon(iconDraft)
    setEditingIcon(false)
    iconButton.current?.focus()
  }

  return (
    <header className="phead">
      {p.coverAssetId !== null && (
        <div
          className="phead__cover"
          style={{ backgroundImage: `url(/assets/${p.coverAssetId})` }}
        />
      )}

      <div className="phead__top">
        <div className="phead__icon">
          <button
            ref={iconButton}
            type="button"
            className="phead__icon-button"
            aria-label={t('wiki.page.icon')}
            aria-expanded={editingIcon}
            onClick={() => {
              setIconDraft(p.icon)
              setEditingIcon((now) => !now)
            }}
          >
            <span aria-hidden="true">{p.icon !== '' ? p.icon : '＋'}</span>
          </button>
          {editingIcon && (
            <div className="phead__icon-edit">
              <input
                ref={iconInput}
                className="field__input"
                aria-label={t('wiki.page.icon')}
                aria-invalid={!iconOk}
                placeholder={t('wiki.page.iconPlaceholder')}
                value={iconDraft}
                onChange={(e) => {
                  setIconDraft(e.target.value)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    endIcon(true)
                  } else if (e.key === 'Escape') {
                    e.preventDefault()
                    e.stopPropagation()
                    endIcon(false)
                  }
                }}
                onBlur={() => {
                  if (iconOk && iconDraft !== p.icon) p.onIcon(iconDraft)
                  setEditingIcon(false)
                }}
              />
              {!iconOk && (
                <p className="field__text field__text--error">{t('wiki.page.iconInvalid')}</p>
              )}
            </div>
          )}
        </div>

        <div className="phead__cover-actions">
          {p.coverAssetId === null ? (
            <button
              type="button"
              className="phead__link"
              onClick={() => {
                setPickingCover(true)
              }}
            >
              {t('wiki.page.cover.add')}
            </button>
          ) : (
            <>
              <button
                type="button"
                className="phead__link"
                onClick={() => {
                  setPickingCover(true)
                }}
              >
                {t('wiki.page.cover.change')}
              </button>
              <button
                type="button"
                className="phead__link"
                onClick={() => {
                  p.onCover(null)
                }}
              >
                {t('wiki.page.cover.remove')}
              </button>
            </>
          )}
        </div>
      </div>

      {p.path.length > 0 && (
        <nav className="phead__path" aria-label={t('wiki.page.path')}>
          {p.path.map((ref) => (
            <span key={ref.id} className="phead__crumb">
              <button
                type="button"
                className="phead__link"
                onClick={() => {
                  p.onOpenPath(ref.id)
                }}
              >
                {[ref.icon, ref.title === '' ? t('wiki.untitled') : ref.title]
                  .filter((s) => s !== '')
                  .join(' ')}
              </button>
              <span aria-hidden="true">/</span>
            </span>
          ))}
        </nav>
      )}

      <div className="phead__row">
        <input
          className="phead__title"
          aria-label={t('wiki.page.title')}
          placeholder={t('wiki.page.titlePlaceholder')}
          value={p.title}
          maxLength={maxTitleLength}
          onChange={(e) => {
            p.onTitle(e.target.value)
          }}
        />
        <SaveIndicator state={p.saveState} error={p.saveError} onRetry={p.onRetry} />
      </div>

      <Dialog
        title={t('wiki.page.cover.title')}
        open={pickingCover}
        closeLabel={t('wiki.page.cover.done')}
        onClose={() => {
          setPickingCover(false)
        }}
      >
        <AssetPicker
          campaignId={p.campaignId}
          kind="other"
          value={p.coverAssetId}
          noneLabel={t('wiki.page.cover.none')}
          onChange={(asset) => {
            p.onCover(asset === null ? null : asset.id)
            setPickingCover(false)
          }}
        />
      </Dialog>
    </header>
  )
}

interface SaveIndicatorProps {
  state: SaveState
  error: unknown
  onRetry: () => void
}

/** SaveIndicator says whether the page is saved; a failure comes with "Retry". A conflict has a banner of its own. */
function SaveIndicator({ state, error, onRetry }: SaveIndicatorProps) {
  const { t } = useTranslation()
  if (state === 'conflict') return <p className="phead__save" role="status" />
  if (state === 'failed') {
    return (
      <p className="phead__save phead__save--failed" role="status" title={t(errorKey(error))}>
        {t('wiki.page.save.failed')}
        <button type="button" className="phead__link" onClick={onRetry}>
          {t('wiki.page.save.retry')}
        </button>
      </p>
    )
  }
  return (
    <p className="phead__save" role="status">
      {state === 'saved' ? t('wiki.page.save.saved') : t('wiki.page.save.saving')}
    </p>
  )
}

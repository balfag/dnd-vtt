import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Campaign } from '../../shared/api/campaigns'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { formatDate } from './format'

interface CampaignListProps {
  campaigns: Campaign[]
  onCreate: (name: string) => Promise<void>
  onRename: (id: string, name: string) => Promise<void>
  onStart: (c: Campaign) => Promise<void>
  fail: (err: unknown) => string
}

const maxNameRunes = 200

function validName(name: string): boolean {
  const n = Array.from(name.trim()).length
  return n >= 1 && n <= maxNameRunes
}

/** CampaignList is the registry of campaigns: one row each, the action at the right. */
export function CampaignList({ campaigns, onCreate, onRename, onStart, fail }: CampaignListProps) {
  const { t, i18n } = useTranslation()
  const active = campaigns.find((c) => c.activeGameSession !== null) ?? null
  const [renaming, setRenaming] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [draft, setDraft] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmStart, setConfirmStart] = useState<Campaign | null>(null)
  const emptyButton = useRef<HTMLButtonElement>(null)
  const draftInput = useRef<HTMLInputElement>(null)
  const newButton = useRef<HTMLButtonElement>(null)
  const rows = useRef(new Map<string, HTMLButtonElement>())
  // Where the focus goes when an edit ends by the keyboard: back to its trigger.
  const returnFocus = useRef<string | null>(null)

  // With no campaigns the only action is the one to create the first: it gets the focus.
  const isEmpty = campaigns.length === 0
  useEffect(() => {
    if (isEmpty) emptyButton.current?.focus()
  }, [isEmpty])

  useEffect(() => {
    if (renaming !== null || creating) {
      draftInput.current?.focus()
      return
    }
    const target = returnFocus.current
    returnFocus.current = null
    if (target === 'new') newButton.current?.focus()
    else if (target !== null) rows.current.get(target)?.focus()
  }, [renaming, creating])

  function openRename(c: Campaign) {
    setCreating(false)
    setRenaming(c.id)
    setDraft(c.name)
    setFieldError(null)
  }

  function openCreate() {
    setRenaming(null)
    setCreating(true)
    setDraft('')
    setFieldError(null)
  }

  // restore: the edit ended by the keyboard, so the focus returns to its trigger.
  function cancelEdit(restore = false) {
    if (restore) returnFocus.current = renaming ?? (creating ? 'new' : null)
    setRenaming(null)
    setCreating(false)
    setFieldError(null)
  }

  async function submit() {
    if (!validName(draft)) {
      setFieldError('campaigns.nameInvalid')
      return
    }
    setBusy(true)
    try {
      if (renaming !== null) await onRename(renaming, draft.trim())
      else await onCreate(draft.trim())
      cancelEdit(true)
    } catch (err) {
      setFieldError(fail(err))
    } finally {
      setBusy(false)
    }
  }

  async function start(c: Campaign) {
    setConfirmStart(null)
    setError(null)
    setBusy(true)
    try {
      await onStart(c)
    } catch (err) {
      setError(fail(err))
    } finally {
      setBusy(false)
    }
  }

  function askStart(c: Campaign) {
    // Starting a game ends the running one: ask first.
    if (active !== null && active.id !== c.id) setConfirmStart(c)
    else void start(c)
  }

  function nameField(label: string) {
    return (
      <div className="registry__edit">
        <input
          ref={draftInput}
          className="field__input"
          aria-label={label}
          aria-invalid={fieldError !== null}
          placeholder={t('campaigns.newPlaceholder')}
          value={draft}
          disabled={busy}
          onChange={(e) => {
            setDraft(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              void submit()
            } else if (e.key === 'Escape') {
              e.preventDefault()
              cancelEdit(true)
            }
          }}
          onBlur={() => {
            if (!busy) cancelEdit()
          }}
        />
        {fieldError !== null && <p className="field__text field__text--error">{t(fieldError)}</p>}
      </div>
    )
  }

  return (
    <section className="registry-section" aria-label={t('campaigns.title')}>
      {error !== null && <ErrorNote>{t(error)}</ErrorNote>}

      {isEmpty && !creating ? (
        <div className="registry__empty">
          <p>{t('campaigns.empty')}</p>
          <Button ref={emptyButton} variant="secondary" fullWidth={false} onClick={openCreate}>
            {t('campaigns.new')}
          </Button>
        </div>
      ) : (
        <div className="registry" role="table" aria-label={t('campaigns.title')}>
          <div className="registry__row registry__row--head" role="row">
            <span role="columnheader">{t('campaigns.colName')}</span>
            <span role="columnheader">{t('campaigns.colCreated')}</span>
            <span role="columnheader" />
          </div>
          {campaigns.map((c) => (
            <div key={c.id} className="registry__row" role="row">
              <span className="registry__name" role="cell">
                {renaming === c.id ? (
                  nameField(t('campaigns.rename', { name: c.name }))
                ) : (
                  <button
                    type="button"
                    ref={(el) => {
                      if (el) rows.current.set(c.id, el)
                      else rows.current.delete(c.id)
                    }}
                    className="text-button text-button--name"
                    aria-label={t('campaigns.rename', { name: c.name })}
                    onClick={() => {
                      openRename(c)
                    }}
                  >
                    {c.name}
                  </button>
                )}
              </span>
              <span className="registry__created" role="cell">
                {formatDate(c.createdAt, i18n.language)}
              </span>
              <span className="registry__action" role="cell">
                {c.activeGameSession !== null ? (
                  <span className="registry__running">{t('campaigns.running')}</span>
                ) : (
                  <Button
                    variant="secondary"
                    fullWidth={false}
                    disabled={busy}
                    onClick={() => {
                      askStart(c)
                    }}
                  >
                    {t('campaigns.start')}
                  </Button>
                )}
              </span>
            </div>
          ))}
          {creating && (
            <div className="registry__row" role="row">
              <span className="registry__name" role="cell">
                {nameField(t('campaigns.new'))}
              </span>
              <span role="cell" />
              <span role="cell" />
            </div>
          )}
        </div>
      )}

      {!isEmpty && !creating && (
        <div className="registry__new">
          <Button ref={newButton} variant="secondary" fullWidth={false} onClick={openCreate}>
            {t('campaigns.new')}
          </Button>
        </div>
      )}

      <Dialog
        open={confirmStart !== null}
        title={t('campaigns.start')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirmStart(null)
        }}
        actions={
          <Button
            fullWidth={false}
            disabled={busy}
            onClick={() => {
              if (confirmStart !== null) void start(confirmStart)
            }}
          >
            {t('campaigns.start')}
          </Button>
        }
      >
        <p>{t('game.replaceConfirm', { name: active?.name ?? '' })}</p>
      </Dialog>
    </section>
  )
}

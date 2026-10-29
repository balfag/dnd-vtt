import { lazy, Suspense, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageType, type PlayerInfo } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { readableColor } from '../chat/playerColor'
import type { HandoutSource } from '../wiki/wikiContext'
import { MAX_HANDOUT_BYTES, toHandout } from './toHandout'
import './handout.css'

// The editor (TipTap and ProseMirror) is a chunk of its own.
const WikiEditor = lazy(() =>
  import('../wiki/editor/WikiEditor').then((m) => ({ default: m.WikiEditor })),
)

/** MAX_TITLE is the longest title the server takes, in characters. */
const MAX_TITLE = 200
/** MAX_RECIPIENTS is the most players one handout may be shown to. */
const MAX_RECIPIENTS = 20

interface ShowHandoutDialogProps {
  open: boolean
  source: HandoutSource
  /** players are the participants of the game; the GM is left out here. */
  players: PlayerInfo[]
  command: (type: string, data?: unknown) => Promise<unknown>
  /** onShown is called after the server took the handout. */
  onShown: () => void
  onClose: () => void
}

/**
 * ShowHandoutDialog asks whom to show a fragment of a page to and under what title, shows how the
 * players will see it, and sends handout.show. Its form starts from `source`: remount it per opening.
 */
export function ShowHandoutDialog({
  open,
  source,
  players,
  command,
  onShown,
  onClose,
}: ShowHandoutDialogProps) {
  const { t } = useTranslation()
  const ids = useId()
  const recipients = useMemo(() => players.filter((p) => p.role === 'player'), [players])
  const [title, setTitle] = useState(source.title.slice(0, MAX_TITLE))
  const [toAll, setToAll] = useState(true)
  const [picked, setPicked] = useState<string[]>([])
  const [failure, setFailure] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const prepared = useMemo(
    () => toHandout(source.doc, source.resolveLabel),
    [source.doc, source.resolveLabel],
  )
  // The command carries the title as well: it must fit the frame together with the content.
  const tooBig = prepared.bytes + new TextEncoder().encode(title).length > MAX_HANDOUT_BYTES
  const chosen = recipients.filter((p) => picked.includes(p.id))
  const needPick = !toAll && chosen.length === 0
  const tooMany = !toAll && chosen.length > MAX_RECIPIENTS

  let sendLabel = t('handout.show.sendAll')
  if (!toAll) {
    const [first] = chosen
    if (chosen.length === 1 && first !== undefined) {
      sendLabel = t('handout.show.sendOne', { name: first.displayName })
    } else if (chosen.length > 1) {
      sendLabel = t('handout.show.sendMany', { count: chosen.length })
    } else {
      sendLabel = t('handout.show.send')
    }
  }

  const problem = tooBig
    ? t('handout.show.tooBig')
    : needPick
      ? t('handout.show.noRecipients')
      : recipients.length === 0
        ? t('handout.show.noPlayers')
        : null

  function send() {
    if (busy || problem !== null || tooMany) return
    setBusy(true)
    setFailure(null)
    const data = {
      title: title.trim(),
      content: prepared.doc,
      ...(toAll ? {} : { to: chosen.map((p) => p.id) }),
    }
    command(MessageType.HandoutShow, data).then(
      () => {
        onShown()
        onClose()
      },
      (err: unknown) => {
        setFailure(commandErrorKey(err))
        setBusy(false)
      },
    )
  }

  return (
    <Dialog
      title={t('handout.show.title')}
      open={open}
      closeLabel={t('ui.dialog.cancel')}
      onClose={onClose}
      actions={
        <Button fullWidth={false} disabled={busy || problem !== null || tooMany} onClick={send}>
          {sendLabel}
        </Button>
      }
    >
      <div className="handout-form">
        <fieldset className="handout-form__to">
          <legend className="handout-form__label">{t('handout.show.to')}</legend>
          <div className="handout-form__row">
            <label className="handout-form__choice">
              <input
                type="radio"
                name={`${ids}-to`}
                checked={toAll}
                onChange={() => {
                  setToAll(true)
                }}
              />
              {t('handout.show.all')}
            </label>
            <label className="handout-form__choice">
              <input
                type="radio"
                name={`${ids}-to`}
                checked={!toAll}
                onChange={() => {
                  setToAll(false)
                }}
              />
              {t('handout.show.selected')}
            </label>
          </div>
          {!toAll && (
            <div className="handout-form__row" role="group" aria-label={t('handout.show.players')}>
              {recipients.map((p) => (
                <label key={p.id} className="handout-form__choice">
                  <input
                    type="checkbox"
                    checked={picked.includes(p.id)}
                    onChange={(e) => {
                      setPicked((now) =>
                        e.target.checked ? [...now, p.id] : now.filter((id) => id !== p.id),
                      )
                    }}
                  />
                  <span
                    className="handout-form__dot"
                    style={{ background: readableColor(p.color) }}
                    aria-hidden="true"
                  />
                  {p.displayName}
                </label>
              ))}
            </div>
          )}
        </fieldset>
        <div className="handout-form__field">
          <label className="handout-form__label" htmlFor={`${ids}-title`}>
            {t('handout.show.titleField')}
          </label>
          <input
            id={`${ids}-title`}
            className="field__input"
            type="text"
            maxLength={MAX_TITLE}
            value={title}
            onChange={(e) => {
              setTitle(e.target.value)
            }}
          />
        </div>
        <div className="handout-form__field">
          <span className="handout-form__label" id={`${ids}-preview`}>
            {t('handout.show.preview')}
          </span>
          <div className="handout-form__preview" role="group" aria-labelledby={`${ids}-preview`}>
            <Suspense fallback={null}>
              <WikiEditor content={prepared.doc} editable={false} />
            </Suspense>
          </div>
        </div>
        {problem !== null && !busy && <p className="handout-form__problem">{problem}</p>}
        {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
      </div>
    </Dialog>
  )
}

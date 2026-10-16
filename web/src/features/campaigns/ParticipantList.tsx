import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CampaignPlayer, PasswordResetLink } from '../../shared/api/campaigns'
import { Button } from '../../ui/Button'
import { CopyField } from '../../ui/CopyField'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { formatMoment } from './format'

interface ParticipantsProps {
  players: CampaignPlayer[]
  /** onReset asks the server for a reset link; the link lives only in the open dialog. */
  onReset: (p: CampaignPlayer) => Promise<PasswordResetLink>
  onRemove: (p: CampaignPlayer) => Promise<void>
  /** fail turns an error into the i18n key of its text. */
  fail: (err: unknown) => string
  notify: (text: string) => void
}

interface ResetDialog {
  player: CampaignPlayer
  link: PasswordResetLink
}

/** Participants lists who plays in the active game and who is online right now. */
export function Participants({ players, onReset, onRemove, fail, notify }: ParticipantsProps) {
  const { t, i18n } = useTranslation()
  const [reset, setReset] = useState<ResetDialog | null>(null)
  const [removing, setRemoving] = useState<CampaignPlayer | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ playerId: string; key: string } | null>(null)

  // The row buttons are not disabled while a request runs: a disabled button
  // loses the focus, and the dialog could not give it back.
  async function openReset(player: CampaignPlayer) {
    if (busy) return
    setError(null)
    setBusy(true)
    try {
      setReset({ player, link: await onReset(player) })
    } catch (err) {
      setError({ playerId: player.id, key: fail(err) })
    } finally {
      setBusy(false)
    }
  }

  async function confirmRemove(player: CampaignPlayer) {
    setBusy(true)
    try {
      await onRemove(player)
      setRemoving(null)
      notify(t('participants.removed'))
    } catch (err) {
      setRemoving(null)
      setError({ playerId: player.id, key: fail(err) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="participants" aria-labelledby="participants-title">
      <h3 className="participants__title" id="participants-title">
        {t('participants.title')}
      </h3>
      {players.length === 0 ? (
        <p className="participants__empty">{t('participants.empty')}</p>
      ) : (
        <ul className="participants__list">
          {players.map((p) => (
            <li key={p.id} className="participant">
              <span
                className="participant__dot"
                style={{ background: p.online ? p.color : 'transparent', borderColor: p.color }}
                aria-hidden="true"
              />
              <span className="participant__name">{p.displayName}</span>
              <span className="participant__login">{p.accountLogin}</span>
              <span className={p.online ? 'participant__online' : 'participant__offline'}>
                {p.online ? t('participants.online') : t('participants.offline')}
              </span>
              <span className="participant__actions">
                {p.role === 'gm' ? (
                  <span className="participant__role">{t('participants.gmRole')}</span>
                ) : (
                  <>
                    <button
                      type="button"
                      className="text-button"
                      onClick={() => {
                        void openReset(p)
                      }}
                    >
                      {t('participants.resetLink')}
                    </button>
                    <button
                      type="button"
                      className="text-button"
                      onClick={() => {
                        setRemoving(p)
                      }}
                    >
                      {t('participants.remove')}
                    </button>
                  </>
                )}
              </span>
              {error?.playerId === p.id && (
                <div className="participant__error">
                  <ErrorNote>{t(error.key)}</ErrorNote>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      <Dialog
        open={reset !== null}
        title={t('participants.resetTitle', { name: reset?.player.displayName ?? '' })}
        onClose={() => {
          setReset(null)
        }}
      >
        {reset !== null && (
          <>
            <CopyField
              id="reset-link"
              label={t('participants.resetLink')}
              value={reset.link.link}
              primary
              onCopied={() => {
                notify(t('ui.copy.done'))
              }}
            />
            <p>
              {t('participants.resetHint', {
                name: reset.player.displayName,
                time: formatMoment(reset.link.expiresAt, i18n.language),
              })}
            </p>
          </>
        )}
      </Dialog>

      <Dialog
        open={removing !== null}
        title={t('participants.remove')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setRemoving(null)
        }}
        actions={
          <Button
            variant="danger"
            fullWidth={false}
            disabled={busy}
            onClick={() => {
              if (removing !== null) void confirmRemove(removing)
            }}
          >
            {t('ui.dialog.confirmRemove')}
          </Button>
        }
      >
        <p>{t('participants.removeConfirm', { name: removing?.displayName ?? '' })}</p>
      </Dialog>
    </section>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import type { Campaign, CampaignPlayer, PasswordResetLink } from '../../shared/api/campaigns'
import { withHost } from '../../shared/api/net'
import { Button } from '../../ui/Button'
import { CopyField } from '../../ui/CopyField'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { formatMoment } from './format'
import { Participants } from './ParticipantList'
import { useLanHost } from './useLanHost'

interface ActiveGameProps {
  campaign: Campaign
  /** link is the game link while this page remembers it; null after a reload. */
  link: string | null
  players: CampaignPlayer[]
  onRotate: () => Promise<void>
  onEnd: () => Promise<void>
  onResetLink: (p: CampaignPlayer) => Promise<PasswordResetLink>
  onRemove: (p: CampaignPlayer) => Promise<void>
  fail: (err: unknown) => string
  notify: (text: string) => void
}

type Confirm = 'rotate' | 'end' | null

/** ActiveGame is the strip above the list: the running game, its link and its participants. */
export function ActiveGame({
  campaign,
  link,
  players,
  onRotate,
  onEnd,
  onResetLink,
  onRemove,
  fail,
  notify,
}: ActiveGameProps) {
  const { t, i18n } = useTranslation()
  const [confirm, setConfirm] = useState<Confirm>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const lan = useLanHost(link)
  const startedAt = campaign.activeGameSession?.startedAt ?? 0

  async function run(action: () => Promise<void>) {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (err) {
      setError(fail(err))
    } finally {
      setBusy(false)
      setConfirm(null)
    }
  }

  return (
    <section className="active-game" aria-labelledby="active-game-title">
      <h2 className="active-game__title" id="active-game-title">
        {t('game.running', {
          name: campaign.name,
          time: formatMoment(startedAt, i18n.language),
        })}
      </h2>

      {link === null ? (
        <p className="active-game__hidden">{t('game.linkHidden')}</p>
      ) : (
        <>
          {lan.host !== null && (
            <div>
              <label className="field__label" htmlFor="lan-host">
                {t('game.lanHost')}
              </label>
              <select
                id="lan-host"
                className="field__input"
                value={lan.host}
                onChange={(e) => {
                  lan.setHost(e.target.value)
                }}
              >
                {lan.hosts.map((h) => (
                  <option key={h} value={h}>
                    {h}
                  </option>
                ))}
              </select>
            </div>
          )}
          <CopyField
            id="game-link"
            label={t('game.linkLabel')}
            value={lan.host === null ? link : withHost(link, lan.host)}
            onCopied={() => {
              notify(t('ui.copy.done'))
            }}
          />
        </>
      )}

      <div className="active-game__buttons">
        <Button
          fullWidth={false}
          onClick={() => {
            navigate('/table')
          }}
        >
          {t('table.open')}
        </Button>
        <Button
          variant="secondary"
          fullWidth={false}
          disabled={busy}
          onClick={() => {
            setConfirm('rotate')
          }}
        >
          {t('game.rotate')}
        </Button>
        <Button
          variant="danger"
          fullWidth={false}
          disabled={busy}
          onClick={() => {
            setConfirm('end')
          }}
        >
          {t('game.end')}
        </Button>
      </div>
      {error !== null && <ErrorNote>{t(error)}</ErrorNote>}

      <Participants
        players={players}
        lanHost={lan.host}
        onReset={onResetLink}
        onRemove={onRemove}
        fail={fail}
        notify={notify}
      />

      <Dialog
        open={confirm === 'rotate'}
        title={t('game.rotate')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirm(null)
        }}
        actions={
          <Button
            fullWidth={false}
            disabled={busy}
            onClick={() => {
              void run(onRotate)
            }}
          >
            {t('ui.dialog.confirmRotate')}
          </Button>
        }
      >
        <p>{t('game.rotateConfirm')}</p>
      </Dialog>

      <Dialog
        open={confirm === 'end'}
        title={t('game.end')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirm(null)
        }}
        actions={
          <Button
            variant="danger"
            fullWidth={false}
            disabled={busy}
            onClick={() => {
              void run(onEnd)
            }}
          >
            {t('ui.dialog.confirmEnd')}
          </Button>
        }
      >
        <p>{t('game.endConfirm')}</p>
      </Dialog>
    </section>
  )
}

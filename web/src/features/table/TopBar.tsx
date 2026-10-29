import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import type { PlayerInfo } from '../../shared/protocol'

interface TopBarProps {
  campaignName: string
  /** sceneName is the scene on the table; null while there is none. */
  sceneName: string | null
  players: PlayerInfo[]
  isGm: boolean
  /** onOpenScenes opens the scenes panel; the GM's scene name is its button. */
  onOpenScenes: () => void
  /** wikiCampaignId is the campaign whose wiki the GM's "Wiki" link opens; absent for a player. */
  wikiCampaignId?: string
  /** onResetLayout puts the default layout of the workspace back; it is absent on a narrow screen, where there is no workspace. */
  onResetLayout?: () => void
  onLeave: () => void
}

function Participant({ player }: { player: PlayerInfo }) {
  const { t } = useTranslation()
  return (
    <li className={player.online ? 'who' : 'who who--away'}>
      <span
        className="who__dot"
        style={player.online ? { background: player.color } : { borderColor: player.color }}
        aria-hidden="true"
      />
      {player.displayName}
      <span className="sr-only">
        {' '}
        {player.online ? t('participants.online') : t('participants.offline')}
      </span>
    </li>
  )
}

/** TopBar is the strip over the table: campaign, scene on the table, participants, leave. */
export function TopBar({
  campaignName,
  sceneName,
  players,
  isGm,
  onOpenScenes,
  wikiCampaignId,
  onResetLayout,
  onLeave,
}: TopBarProps) {
  const { t } = useTranslation()
  const first = players[0]
  const rest = players.length - 1
  return (
    <header className="topbar">
      <h1 className="topbar__campaign">{campaignName}</h1>
      {isGm ? (
        <button
          type="button"
          className="topbar__scene topbar__scene--button"
          onClick={onOpenScenes}
        >
          {sceneName ?? t('scenes.tab')}
          <span aria-hidden="true"> ▾</span>
        </button>
      ) : (
        sceneName !== null && <p className="topbar__scene">{sceneName}</p>
      )}

      {/* Wide screens list everyone; narrow ones the first participant and "+N". */}
      <ul className="topbar__players topbar__players--all" aria-label={t('table.participants')}>
        {players.map((p) => (
          <Participant key={p.id} player={p} />
        ))}
      </ul>
      {first !== undefined && (
        <ul className="topbar__players topbar__players--first" aria-label={t('table.participants')}>
          <Participant player={first} />
          {rest > 0 && <li className="who who--away">{t('table.more', { count: rest })}</li>}
        </ul>
      )}

      {isGm && wikiCampaignId !== undefined && wikiCampaignId !== '' && (
        <button
          type="button"
          className="topbar__button"
          onClick={() => {
            navigate(`/wiki/${wikiCampaignId}`)
          }}
        >
          {t('wiki.link')}
        </button>
      )}
      {onResetLayout !== undefined && (
        <button type="button" className="topbar__button" onClick={onResetLayout}>
          {t('workspace.reset')}
        </button>
      )}
      <button type="button" className="topbar__button" onClick={onLeave}>
        {isGm ? t('table.leave') : t('common.signOut')}
      </button>
    </header>
  )
}

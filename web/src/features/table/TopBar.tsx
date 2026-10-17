import { useTranslation } from 'react-i18next'
import type { PlayerInfo } from '../../shared/protocol'

interface TopBarProps {
  campaignName: string
  /** sceneName is the scene on the table; null while there is none. */
  sceneName: string | null
  players: PlayerInfo[]
  isGm: boolean
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
export function TopBar({ campaignName, sceneName, players, isGm, onLeave }: TopBarProps) {
  const { t } = useTranslation()
  const first = players[0]
  const rest = players.length - 1
  return (
    <header className="topbar">
      <h1 className="topbar__campaign">{campaignName}</h1>
      {sceneName !== null && <p className="topbar__scene">{sceneName}</p>}

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

      <button type="button" className="topbar__button" onClick={onLeave}>
        {isGm ? t('table.leave') : t('common.signOut')}
      </button>
    </header>
  )
}

import { useTranslation } from 'react-i18next'
import type { ChatMessage, PlayerInfo } from '../../shared/protocol'
import { useGame } from '../game/store'
import { firstImageAsset, handoutAudience } from './audience'
import { openHandout } from './useHandoutQueue'
import './handout.css'

interface HandoutRowProps {
  message: ChatMessage
  players: PlayerInfo[]
  time: string
}

/** HandoutRow is the row of a handout in the chat: who showed it, to whom, a thumbnail or a scroll, the title and "Open". */
export function HandoutRow({ message, players, time }: HandoutRowProps) {
  const { t } = useTranslation()
  const viewerId = useGame((s) => s.me?.playerId ?? null)
  const handout = message.handout
  if (handout === undefined) return null
  const audience = handoutAudience(message, players, viewerId)
  const image = firstImageAsset(handout.content)
  const title = handout.title === '' ? t('handout.untitled') : handout.title

  return (
    <li className={audience.kind === 'everyone' ? 'msg' : 'msg msg--private'} title={time}>
      <div className="msg__head">
        <span className="msg__author">{t('handout.row.author')}</span>
        {audience.kind === 'me' && <span className="msg__meta">{t('handout.row.privateMe')}</span>}
        {audience.kind === 'names' && (
          <span className="msg__meta">{t('handout.row.privateTo', { names: audience.names })}</span>
        )}
      </div>
      <div className="msg__handout">
        {image === null ? (
          <span className="msg__handout-glyph" role="img" aria-label={t('handout.row.icon')}>
            📜
          </span>
        ) : (
          <img className="msg__handout-icon" src={`/assets/${image}`} alt="" />
        )}
        <span className="msg__handout-title">{title}</span>
        <button
          type="button"
          className="msg__handout-open"
          aria-label={t('handout.row.openLabel', { title })}
          onClick={() => {
            openHandout(message)
          }}
        >
          {t('handout.row.open')}
        </button>
      </div>
    </li>
  )
}

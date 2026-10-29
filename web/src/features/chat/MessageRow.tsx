import { useTranslation } from 'react-i18next'
import type { ChatMessage, PlayerInfo } from '../../shared/protocol'
import { HandoutRow } from '../handout/HandoutRow'
import { readableColor } from './playerColor'
import { RollView } from './RollView'

interface MessageRowProps {
  message: ChatMessage
  players: PlayerInfo[]
}

/** MessageRow is one entry of the log: a text, a system note, a roll or a handout. */
export function MessageRow({ message, players }: MessageRowProps) {
  const { t, i18n } = useTranslation()
  const author = players.find((p) => p.id === message.authorPlayerId)
  const time = new Date(message.createdAt).toLocaleTimeString(i18n.language)

  if (message.kind === 'system') {
    return (
      <li className="msg msg--system" title={time}>
        {message.text}
      </li>
    )
  }

  if (message.kind === 'handout') {
    return <HandoutRow message={message} players={players} time={time} />
  }

  const isPrivate = message.visibility === 'private'
  const names = (message.to ?? [])
    .map((id) => players.find((p) => p.id === id)?.displayName ?? '')
    .filter((name) => name !== '')
    .join(', ')
  const visibility =
    message.visibility === 'gm' || message.visibility === 'self' || message.visibility === 'blind'
      ? message.visibility
      : null

  return (
    <li className={isPrivate ? 'msg msg--private' : 'msg'} title={time}>
      <div className="msg__head">
        <span
          className="msg__author"
          style={author === undefined ? undefined : { color: readableColor(author.color) }}
        >
          {author?.displayName ?? ''}
        </span>
        {message.roll !== undefined && message.roll.label !== '' && (
          <span className="msg__meta">{message.roll.label}</span>
        )}
        {isPrivate && <span className="msg__meta">{t('chat.private', { names })}</span>}
        {visibility !== null && <span className="msg__vis">{t(`chat.vis.${visibility}`)}</span>}
      </div>
      {message.roll === undefined ? (
        <p className="msg__text">{message.text}</p>
      ) : (
        <RollView roll={message.roll} />
      )}
    </li>
  )
}

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { isChatHistoryAck, MessageType } from '../../shared/protocol'
import { gameStore, historyPageSize, useGame } from '../game/store'
import { Composer } from './Composer'
import { MessageRow } from './MessageRow'
import './chat.css'

/** The log counts as scrolled to the bottom within this many pixels. */
const bottomSlack = 24
/** Older messages are asked for when the top is closer than this. */
const topReach = 200

interface ChatPanelProps {
  command: (type: string, data?: unknown) => Promise<unknown>
}

/** ChatPanel is the log of messages and rolls, the quick rolls and the input. */
export function ChatPanel({ command }: ChatPanelProps) {
  const { t } = useTranslation()
  const messages = useGame((s) => s.game?.chat)
  const players = useGame((s) => s.game?.players)
  const historyDone = useGame((s) => s.game?.chatHistoryDone ?? true)
  const meId = useGame((s) => s.me?.playerId ?? '')
  const logRef = useRef<HTMLOListElement>(null)
  const atBottom = useRef(true)
  const loading = useRef(false)
  const seen = useRef<{ first: string | null; last: string | null; height: number } | null>(null)
  const [hasNew, setHasNew] = useState(false)

  const list = messages ?? []
  const everyone = players ?? []
  const firstId = list[0]?.id ?? null
  const last = list[list.length - 1]
  const lastId = last?.id ?? null
  const lastAuthor = last?.authorPlayerId

  const loadHistory = useCallback(() => {
    if (loading.current || firstId === null) return
    loading.current = true
    command(MessageType.ChatHistory, { before: firstId, limit: historyPageSize })
      .then((d) => {
        const ack = { t: MessageType.Ack, re: '', d }
        if (isChatHistoryAck(ack)) gameStore.getState().prependHistory(ack.d.messages)
      })
      .catch(() => {
        // The next scroll to the top asks again.
      })
      .finally(() => {
        loading.current = false
      })
  }, [command, firstId])

  // Keep the view steady when the list changes: stay at the bottom, or keep the place after a prepend.
  useLayoutEffect(() => {
    const log = logRef.current
    if (log === null) return
    const before = seen.current
    seen.current = { first: firstId, last: lastId, height: log.scrollHeight }
    if (before === null) {
      log.scrollTop = log.scrollHeight
      return
    }
    if (firstId !== before.first && lastId === before.last) {
      log.scrollTop += log.scrollHeight - before.height
    } else if (lastId !== before.last) {
      if (atBottom.current || lastAuthor === meId) {
        log.scrollTop = log.scrollHeight
        atBottom.current = true
      } else {
        setHasNew(true)
      }
    }
  }, [firstId, lastId, lastAuthor, meId])

  // A log that does not fill the view has no scroll to ask for more with.
  useEffect(() => {
    const log = logRef.current
    if (log !== null && !historyDone && log.scrollTop < topReach) loadHistory()
  }, [historyDone, loadHistory, list.length])

  function onScroll() {
    const log = logRef.current
    if (log === null) return
    atBottom.current = log.scrollHeight - log.scrollTop - log.clientHeight < bottomSlack
    if (atBottom.current) setHasNew(false)
    if (log.scrollTop < topReach && !historyDone) loadHistory()
  }

  function toBottom() {
    const log = logRef.current
    if (log === null) return
    log.scrollTop = log.scrollHeight
    atBottom.current = true
    setHasNew(false)
  }

  return (
    <div className="chat">
      <div className="chat__scroll">
        <ol
          ref={logRef}
          className="chat__log"
          role="log"
          aria-live="polite"
          aria-label={t('chat.tab')}
          onScroll={onScroll}
        >
          {list.map((m) => (
            <MessageRow key={m.id} message={m} players={everyone} />
          ))}
        </ol>
        {list.length === 0 && <p className="chat__empty">{t('chat.empty')}</p>}
      </div>
      {hasNew && (
        <button type="button" className="chat__new" onClick={toBottom}>
          {t('chat.newMessages')}
        </button>
      )}
      <Composer command={command} players={everyone} meId={meId} />
    </div>
  )
}

import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useGame } from '../game/store'

const showAfterMs = 1000

/** ConnectionNote says the link is down, but only once it has been down for a second. */
export function ConnectionNote() {
  const { t } = useTranslation()
  const state = useGame((s) => s.connection.state)
  const down = state === 'connecting' || state === 'reconnecting'
  const [late, setLate] = useState(false)

  useEffect(() => {
    if (!down) return
    const timer = setTimeout(() => {
      setLate(true)
    }, showAfterMs)
    return () => {
      clearTimeout(timer)
      setLate(false)
    }
  }, [down])

  if (!down || !late) return null
  return (
    <p className="connection-note" role="status">
      {t('table.connection.reconnecting')}
    </p>
  )
}

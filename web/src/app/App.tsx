import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchHealth } from '../shared/api/health'

type HealthState = { kind: 'loading' } | { kind: 'ok'; version: string } | { kind: 'error' }

export function App() {
  const { t } = useTranslation()
  const [health, setHealth] = useState<HealthState>({ kind: 'loading' })

  useEffect(() => {
    const controller = new AbortController()
    fetchHealth(controller.signal).then(
      (h) => {
        setHealth({ kind: 'ok', version: h.version })
      },
      () => {
        if (!controller.signal.aborted) {
          setHealth({ kind: 'error' })
        }
      },
    )
    return () => {
      controller.abort()
    }
  }, [])

  return (
    <main>
      <h1>{t('app.title')}</h1>
      <p>
        {health.kind === 'loading' && t('app.health.loading')}
        {health.kind === 'ok' && t('app.health.ok', { version: health.version })}
        {health.kind === 'error' && t('app.health.error')}
      </p>
    </main>
  )
}

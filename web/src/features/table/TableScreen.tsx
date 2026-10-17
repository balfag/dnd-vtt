import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { GameConnection } from '../../shared/ws/connection'
import { gameSocketUrl } from '../../shared/ws/socket'
import { AuthLayout } from '../../ui/AuthLayout'
import { Link } from '../../ui/Link'
import { gameStore, useGame } from '../game/store'
import { SceneCanvas } from './canvas/SceneCanvas'
import { ConnectionNote } from './ConnectionNote'
import { TableAnnouncer, type Announcement } from './TableAnnouncer'
import { ToolRail } from './ToolRail'
import { TopBar } from './TopBar'
import './table.css'

declare global {
  interface Window {
    /** dndvtt exists only in the development build: it lets a developer send commands from the console. */
    dndvtt?: { command: (t: string, d?: unknown) => Promise<unknown> }
  }
}

/** TableScreen is the table: the game connection, the top bar, the tools and the scene canvas. */
export default function TableScreen() {
  const { t } = useTranslation()
  const { state, refresh, logout } = useSession()
  const signedIn = state.status === 'signedIn'
  const anonymous = state.status === 'anonymous'

  useEffect(() => {
    if (anonymous) navigate('/login', { replace: true })
  }, [anonymous])

  const connection = useMemo(
    () =>
      signedIn
        ? new GameConnection({ store: gameStore, url: gameSocketUrl(window.location) })
        : null,
    [signedIn],
  )
  const [announcement, setAnnouncement] = useState<Announcement | null>(null)
  const announce = useCallback((text: string, visible: boolean) => {
    setAnnouncement((prev) => ({ id: (prev?.id ?? 0) + 1, text, visible }))
  }, [])

  useEffect(() => {
    if (connection === null) return
    gameStore.getState().reset()
    connection.start()
    if (import.meta.env.DEV) {
      window.dndvtt = {
        command: (type, data) => connection.command(type, data),
      }
    }
    return () => {
      connection.stop()
      if (import.meta.env.DEV) delete window.dndvtt
    }
  }, [connection])

  const link = useGame((s) => s.connection)
  const snapshotMe = useGame((s) => s.me)
  const game = useGame((s) => s.game)
  const closedReason = link.state === 'closed' ? link.reason : null
  const synced = link.state === 'synced'
  const isGm = state.status === 'signedIn' && state.me.isGm
  // A synced connection without a game: the game is over, or has not started.
  const noGame = synced && game === null

  useEffect(() => {
    if (closedReason === 'unauthorized') void refresh()
  }, [closedReason, refresh])

  useEffect(() => {
    if (noGame && isGm) navigate('/', { replace: true })
  }, [noGame, isGm])

  if (state.status !== 'signedIn') return null

  if (closedReason === 'gameEnded' || (noGame && !isGm)) {
    return (
      <AuthLayout title={t('table.ended.title')}>
        <p>{t('table.ended.text')}</p>
        <p>
          <Link to="/">{t('notFound.home')}</Link>
        </p>
      </AuthLayout>
    )
  }
  if (closedReason === 'removed') {
    return (
      <AuthLayout title={t('table.removed.title')}>
        <p>{t('table.removed.text')}</p>
        <p>
          <Link to="/">{t('notFound.home')}</Link>
        </p>
      </AuthLayout>
    )
  }
  if (closedReason === 'unauthorized') return null

  const scene = game?.scene ?? null
  const emptyText = isGm ? t('table.empty.gm') : t('table.empty.player')

  return (
    <div className="table">
      <TopBar
        campaignName={game?.campaign.name ?? ''}
        sceneName={scene?.name ?? null}
        players={game?.players ?? []}
        isGm={isGm}
        onLeave={() => {
          if (isGm) navigate('/')
          else void logout()
        }}
      />
      <ToolRail />
      <main className="table__stage">
        <SceneCanvas
          scene={scene}
          emptyText={emptyText}
          players={game?.players ?? []}
          me={{ isGM: isGm, playerId: snapshotMe?.playerId ?? null }}
          connection={connection}
          announce={announce}
        />
        <ConnectionNote />
        <TableAnnouncer announcement={announcement} />
        {game !== null && scene === null && <p className="table__empty">{emptyText}</p>}
      </main>
    </div>
  )
}

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { GameConnection } from '../../shared/ws/connection'
import { MessageType, type Token } from '../../shared/protocol'
import { CommandError, commandErrorKey } from '../../shared/ws/errors'
import { gameSocketUrl } from '../../shared/ws/socket'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { Link } from '../../ui/Link'
import { ChatPanel } from '../chat/ChatPanel'
import { FogToolbar, type FogToolState } from '../fog/FogToolbar'
import { gameStore, useGame, viewScene } from '../game/store'
import { ScenesPanel } from '../scenes/ScenesPanel'
import { PlaceTokenPopover } from '../tokens/PlaceTokenPopover'
import { runTokenAction, type TokenAction } from '../tokens/tokenActions'
import { TokenContextMenu } from '../tokens/TokenContextMenu'
import { TokenDialog } from '../tokens/TokenDialog'
import { SceneCanvas, type PlaceToolState } from './canvas/SceneCanvas'
import { ConnectionNote } from './ConnectionNote'
import { PrepFrame } from './PrepFrame'
import { SectionTabs, type Section } from './SectionTabs'
import { SidePanel } from './SidePanel'
import { TableAnnouncer, type Announcement } from './TableAnnouncer'
import { useNarrow } from './useNarrow'
import { ToolRail, type TableTool } from './ToolRail'
import { TopBar } from './TopBar'
import './table.css'

declare global {
  interface Window {
    /** dndvtt exists only in the development build: it lets a developer send commands from the console. */
    dndvtt?: { command: (t: string, d?: unknown) => Promise<unknown> }
  }
}

/**
 * useKeepSceneView keeps the GM's canvas on a scene: after a reconnect it asks for the scene
 * the GM looked at, and when the viewed scene is gone it asks for the active one.
 */
function useKeepSceneView(
  command: (type: string, data?: unknown) => Promise<unknown>,
  on: boolean,
) {
  const game = useGame((s) => s.game)
  const scenes = game?.scenes
  const activeId = game?.activeSceneId ?? null
  const hasScene = game?.scene != null
  useEffect(() => {
    if (!on || scenes === undefined) return
    const known = (id: string | null): id is string =>
      id !== null && scenes.some((s) => s.id === id)
    const before = gameStore.getState().viewBeforeSync
    let target: string | null = null
    if (before !== null) {
      gameStore.getState().clearViewBeforeSync()
      if (before !== activeId && known(before)) target = before
    } else if (!hasScene && known(activeId)) {
      target = activeId
    }
    // A failed scene.view leaves the canvas as it is: the GM can pick a scene in the panel.
    if (target !== null) viewScene(command, target).catch(() => undefined)
  }, [on, command, scenes, activeId, hasScene])
}

/** typesText tells that the key goes into a text field, a dialog or a menu rather than to a tool. */
function typesText(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable || target.closest('dialog') !== null) return true
  if (target.tagName === 'TEXTAREA' || target.tagName === 'SELECT') return true
  return (
    target instanceof HTMLInputElement &&
    !['radio', 'checkbox', 'range', 'button'].includes(target.type)
  )
}

/** TableScreen is the table: the game connection, the top bar, the tools and the scene canvas. */
export default function TableScreen() {
  const { t } = useTranslation()
  const { state, refresh, logout } = useSession()
  const signedIn = state.status === 'signedIn'
  const anonymous = state.status === 'anonymous'
  const connectionRef = useRef<GameConnection | null>(null)
  const [section, setSection] = useState<Section>('map')
  // The tab the side panel shows; undefined leaves the choice to the panel.
  const [panelTab, setPanelTab] = useState<string | undefined>(undefined)
  const [unread, setUnread] = useState(0)
  const narrow = useNarrow()
  const [tool, setTool] = useState<TableTool>('select')
  // The image of the tokens the place tool puts; it is kept while the tool is in hand.
  const [placeImage, setPlaceImage] = useState<PlaceToolState['image']>(null)
  // The token a dialog works on, as it was when the dialog opened; open goes false first, so the focus can return.
  const [editing, setEditing] = useState<{ token: Token; key: number; open: boolean } | null>(null)
  const [menu, setMenu] = useState<{ tokenId: string; at: { x: number; y: number } } | null>(null)
  const [deleting, setDeleting] = useState<{ token: Token; open: boolean } | null>(null)
  const [fogOptions, setFogOptions] = useState<Pick<FogToolState, 'shape' | 'radiusCells'>>({
    shape: 'rect',
    radiusCells: 1,
  })

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
    connectionRef.current = connection
    if (import.meta.env.DEV) {
      window.dndvtt = {
        command: (type, data) => connection.command(type, data),
      }
    }
    return () => {
      connection.stop()
      connectionRef.current = null
      if (import.meta.env.DEV) delete window.dndvtt
    }
  }, [connection])

  const command = useCallback((type: string, data?: unknown): Promise<unknown> => {
    return (
      connectionRef.current?.command(type, data) ?? Promise.reject(new CommandError('disconnected'))
    )
  }, [])

  const link = useGame((s) => s.connection)
  const snapshotMe = useGame((s) => s.me)
  const game = useGame((s) => s.game)
  const closedReason = link.state === 'closed' ? link.reason : null
  const synced = link.state === 'synced'
  const isGm = state.status === 'signedIn' && state.me.isGm
  // A synced connection without a game: the game is over, or has not started.
  const noGame = synced && game === null

  useKeepSceneView(command, isGm && synced)

  // Tool keys (V, K, F, Shift+F); the place tool and the fog tools are the GM's.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || typesText(e.target)) return
      // Escape lets go of the place tool; the canvas takes the same key to drop the selection.
      if (e.key === 'Escape') {
        setTool((now) => (now === 'place' ? 'select' : now))
        return
      }
      const key = e.key.toLowerCase()
      if (key === 'v') {
        setTool('select')
      } else if (key === 'k' && isGm) {
        setTool((now) => (now === 'place' ? 'select' : 'place'))
      } else if (key === 'f' && isGm) {
        const next = e.shiftKey ? 'hide' : 'reveal'
        setTool((now) => (now === next ? 'select' : next))
      } else {
        return
      }
      e.preventDefault()
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [isGm])

  // Count what the chat gets while the chat is not shown on a narrow screen.
  useEffect(() => {
    if (!narrow || section === 'chat') return
    return gameStore.subscribe((now, before) => {
      const last = now.game?.chat[now.game.chat.length - 1]
      if (last === undefined || last.id === before.game?.chat[before.game.chat.length - 1]?.id)
        return
      if (last.authorPlayerId !== now.me?.playerId) setUnread((n) => n + 1)
    })
  }, [narrow, section])

  useEffect(() => {
    if (closedReason === 'unauthorized') void refresh()
  }, [closedReason, refresh])

  useEffect(() => {
    if (noGame && isGm) navigate('/', { replace: true })
  }, [noGame, isGm])

  if (state.status !== 'signedIn') return null

  /** doTokenAction does an action of the GM on a token: a dialog, a confirmation, or a command whose result is announced. */
  function doTokenAction(action: TokenAction, tokenId: string) {
    const token = (game?.scene?.tokens ?? []).find((tk) => tk.id === tokenId)
    if (token === undefined) return
    if (action === 'properties') {
      setEditing((e) => ({ token, key: (e?.key ?? 0) + 1, open: true }))
    } else if (action === 'delete') {
      setDeleting({ token, open: true })
    } else {
      runTokenAction(action, token, game?.scene?.tokens ?? [], command).then(
        (announcement) => {
          announce(t(announcement), false)
        },
        (err: unknown) => {
          announce(t(commandErrorKey(err)), true)
        },
      )
    }
  }

  function deleteToken(tokenId: string) {
    setDeleting((d) => (d === null ? null : { ...d, open: false }))
    command(MessageType.TokenDelete, { tokenId }).then(
      () => {
        announce(t('tokens.announce.deleted'), false)
      },
      (err: unknown) => {
        announce(t(commandErrorKey(err)), true)
      },
    )
  }

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
  const preparing = isGm && scene !== null && scene.id !== game?.activeSceneId
  const fogTool: FogToolState | null =
    isGm && scene !== null && (tool === 'reveal' || tool === 'hide')
      ? { mode: tool, ...fogOptions }
      : null
  const placeTool: PlaceToolState | null =
    isGm && scene !== null && tool === 'place' ? { image: placeImage } : null
  const campaignId = game?.campaign.id ?? ''
  const tokens = scene?.tokens ?? []
  const menuToken = menu === null ? null : (tokens.find((tk) => tk.id === menu.tokenId) ?? null)
  const emptyText = isGm ? t('table.empty.gm') : t('table.empty.player')

  return (
    <div className="table" data-section={section}>
      <TopBar
        campaignName={game?.campaign.name ?? ''}
        sceneName={scene?.name ?? null}
        players={game?.players ?? []}
        isGm={isGm}
        onOpenScenes={() => {
          setPanelTab('scenes')
        }}
        onLeave={() => {
          if (isGm) navigate('/')
          else void logout()
        }}
      />
      <ToolRail tool={isGm ? tool : 'select'} onTool={setTool} isGm={isGm} />
      <main className="table__stage" inert={narrow && section !== 'map'}>
        <SceneCanvas
          scene={scene}
          emptyText={emptyText}
          players={game?.players ?? []}
          me={{ isGM: isGm, playerId: snapshotMe?.playerId ?? null }}
          connection={connection}
          announce={announce}
          fogTool={fogTool}
          campaignId={campaignId}
          placeTool={placeTool}
          onOpenToken={(id) => {
            doTokenAction('properties', id)
          }}
          onTokenMenu={(id, at) => {
            setMenu({ tokenId: id, at })
          }}
          onTokenAction={doTokenAction}
        />
        {placeTool !== null && (
          <PlaceTokenPopover
            campaignId={campaignId}
            assetId={placeImage?.assetId ?? null}
            onChange={(asset) => {
              setPlaceImage(
                asset === null ? null : { assetId: asset.id, originalName: asset.originalName },
              )
            }}
            onClose={() => {
              setTool('select')
              document.querySelector<HTMLElement>('[data-tool="place"]')?.focus()
            }}
          />
        )}
        {fogTool !== null && scene !== null && (
          <FogToolbar
            scene={scene}
            tool={fogTool}
            onChange={(next) => {
              setFogOptions({ shape: next.shape, radiusCells: next.radiusCells })
            }}
            command={command}
            announce={announce}
          />
        )}
        {preparing && (
          <PrepFrame
            viewedName={scene.name}
            hasActive={game?.activeSceneId != null}
            onShow={async () => {
              await command(MessageType.SceneActivate, { sceneId: scene.id })
            }}
          />
        )}
        {menu !== null && menuToken !== null && (
          <TokenContextMenu
            token={menuToken}
            at={menu.at}
            onAction={(action) => {
              setMenu(null)
              doTokenAction(action, menuToken.id)
            }}
            onClose={() => {
              setMenu(null)
              document.querySelector<HTMLElement>('.canvas__host')?.focus()
            }}
          />
        )}
        {editing !== null && (
          <TokenDialog
            key={editing.key}
            open={editing.open}
            original={editing.token}
            players={game?.players ?? []}
            campaignId={campaignId}
            command={command}
            onSaved={() => {
              announce(t('tokens.announce.saved'), false)
            }}
            onClose={() => {
              setEditing((e) => (e === null ? null : { ...e, open: false }))
            }}
          />
        )}
        {deleting !== null && (
          <Dialog
            title={t('tokens.menu.delete')}
            open={deleting.open}
            closeLabel={t('ui.dialog.cancel')}
            onClose={() => {
              setDeleting((d) => (d === null ? null : { ...d, open: false }))
            }}
            actions={
              <Button
                variant="danger"
                fullWidth={false}
                onClick={() => {
                  deleteToken(deleting.token.id)
                }}
              >
                {t('tokens.menu.delete')}
              </Button>
            }
          >
            <p>
              {deleting.token.name === ''
                ? t('tokens.deleteConfirmUnnamed')
                : t('tokens.deleteConfirm', { name: deleting.token.name })}
            </p>
          </Dialog>
        )}
        <ConnectionNote />
        <TableAnnouncer announcement={announcement} />
        {game !== null && scene === null && <p className="table__empty">{emptyText}</p>}
      </main>
      <SidePanel
        tabs={[
          { id: 'chat', label: t('chat.tab'), content: <ChatPanel command={command} /> },
          ...(isGm
            ? [{ id: 'scenes', label: t('scenes.tab'), content: <ScenesPanel command={command} /> }]
            : []),
        ]}
        selected={panelTab}
        onSelect={(id) => {
          setPanelTab(id)
          if (narrow) setSection(id === 'scenes' ? 'scenes' : 'chat')
        }}
      />
      <SectionTabs
        isGm={isGm}
        selected={section}
        unread={unread}
        onSelect={(next) => {
          setSection(next)
          if (next !== 'map') setPanelTab(next)
          if (next === 'chat') setUnread(0)
        }}
      />
    </div>
  )
}

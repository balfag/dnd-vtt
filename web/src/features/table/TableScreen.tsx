import type { DockviewApi, IDockviewPanel } from 'dockview-react'
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
import { CombatPanel } from '../combat/CombatPanel'
import { turnChange } from '../combat/turn'
import { FogToolbar, type FogToolState } from '../fog/FogToolbar'
import { gameStore, useGame, viewScene } from '../game/store'
import { ScenesPanel } from '../scenes/ScenesPanel'
import { ToolOptions } from '../tools/ToolOptions'
import {
  DEFAULT_DRAW,
  GM_TOOLS,
  DEFAULT_TEMPLATE,
  type DrawOptions,
  type TemplateOptions,
} from '../tools/toolState'
import { PlaceTokenPopover } from '../tokens/PlaceTokenPopover'
import { runTokenAction, type TokenAction } from '../tokens/tokenActions'
import { TokenContextMenu } from '../tokens/TokenContextMenu'
import { TokenDialog } from '../tokens/TokenDialog'
import { layoutKey } from '../workspace/layout'
import { panelOptions, type PanelDef } from '../workspace/panels'
import { Workspace } from '../workspace/Workspace'
import { useWorkspaceOpen, useWorkspaceReset } from '../workspace/workspaceCommands'
import { SceneCanvas, type CanvasTool, type PlaceToolState } from './canvas/SceneCanvas'
import { ConnectionNote } from './ConnectionNote'
import { PrepFrame } from './PrepFrame'
import { SectionTabs, type Section } from './SectionTabs'
import { SidePanel } from './SidePanel'
import { TableAnnouncer, type Announcement } from './TableAnnouncer'
import { useNarrow } from './useNarrow'
import { ToolRail, type TableTool } from './ToolRail'
import { TokenList } from './TokenList'
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

/** sharedToolKeys are the keys of the tools every participant has. */
const sharedToolKeys: Partial<Record<string, TableTool>> = {
  r: 'ruler',
  p: 'ping',
  d: 'draw',
  t: 'template',
}

/** SIDE_WIDTH_REM is the width of the group at the right of the canvas in the default layout. */
const SIDE_WIDTH_REM = 22

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
  const resetLayout = useWorkspaceReset()
  const openPanel = useWorkspaceOpen()
  const [tool, setTool] = useState<TableTool>('select')
  // The image of the tokens the place tool puts; it is kept while the tool is in hand.
  const [placeImage, setPlaceImage] = useState<PlaceToolState['image']>(null)
  // The token a dialog works on, as it was when the dialog opened; open goes false first, so the focus can return.
  const [editing, setEditing] = useState<{ token: Token; key: number; open: boolean } | null>(null)
  const [menu, setMenu] = useState<{ tokenId: string; at: { x: number; y: number } } | null>(null)
  const [deleting, setDeleting] = useState<{ token: Token; open: boolean } | null>(null)
  const [drawOptions, setDrawOptions] = useState<DrawOptions>(DEFAULT_DRAW)
  const [templateOptions, setTemplateOptions] = useState<TemplateOptions>(DEFAULT_TEMPLATE)
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

  // Tool keys (V, R, P, D, T, and K, F, Shift+F for the GM); the place tool and the fog tools are the GM's.
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
      } else if (key in sharedToolKeys) {
        const next = sharedToolKeys[key]
        if (next === undefined) return
        setTool((now) => (now === next ? 'select' : next))
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

  // A new turn is read aloud; the viewer's own turn says so.
  useEffect(() => {
    return gameStore.subscribe((now, before) => {
      if (now.game?.combat === before.game?.combat) return
      const me = { isGM: now.me?.isGm === true, playerId: now.me?.playerId ?? null }
      const change = turnChange(
        before.game?.combat ?? null,
        now.game?.combat ?? null,
        now.game?.scene?.tokens ?? [],
        me,
      )
      if (change === null) return
      const name = change.name === '' ? t('combat.unnamed') : change.name
      announce(change.mine ? t('combat.announceMyTurn') : t('combat.announceTurn', { name }), false)
    })
  }, [announce, t])

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
  // A player holds only the tools of everyone.
  const shownTool: TableTool = isGm || !GM_TOOLS.includes(tool) ? tool : 'select'
  const canvasTool: CanvasTool | null =
    scene !== null &&
    (shownTool === 'ruler' ||
      shownTool === 'ping' ||
      shownTool === 'draw' ||
      shownTool === 'template')
      ? shownTool
      : null
  const myColor =
    (game?.players ?? []).find((p) => p.id === snapshotMe?.playerId)?.color ?? '#e53935'
  const placeTool: PlaceToolState | null =
    isGm && scene !== null && tool === 'place' ? { image: placeImage } : null
  const campaignId = game?.campaign.id ?? ''
  const tokens = scene?.tokens ?? []
  const menuToken = menu === null ? null : (tokens.find((tk) => tk.id === menu.tokenId) ?? null)
  const emptyText = isGm ? t('table.empty.gm') : t('table.empty.player')

  const accountId = state.me.accountId
  const stage = (
    <>
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
        tool={canvasTool}
        drawOptions={drawOptions}
        templateOptions={templateOptions}
      />
      {canvasTool !== null && (
        <ToolOptions
          tool={canvasTool}
          isGm={isGm}
          draw={drawOptions}
          onDraw={setDrawOptions}
          template={templateOptions}
          onTemplate={setTemplateOptions}
          myColor={myColor}
        />
      )}
      <TokenList scene={scene} />
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
    </>
  )
  const toolRail = <ToolRail tool={shownTool} onTool={setTool} isGm={isGm} />

  const topBar = (
    <TopBar
      campaignName={game?.campaign.name ?? ''}
      sceneName={scene?.name ?? null}
      players={game?.players ?? []}
      isGm={isGm}
      onOpenScenes={() => {
        if (narrow) setPanelTab('scenes')
        else openPanel('scenes')
      }}
      onResetLayout={
        narrow
          ? undefined
          : () => {
              resetLayout(layoutKey(accountId, campaignId, 'table'))
            }
      }
      onLeave={() => {
        if (isGm) navigate('/')
        else void logout()
      }}
    />
  )

  if (!narrow) {
    // The catalogue of the panels of the table (docs/architecture/workspace.md).
    const catalogue: PanelDef[] = [
      {
        kind: 'scene',
        title: t('table.section.map'),
        closable: false,
        popout: false,
        gmOnly: false,
        render: () => (
          <div className="scenepanel">
            {toolRail}
            <main className="table__stage">{stage}</main>
          </div>
        ),
      },
      {
        kind: 'chat',
        title: t('chat.tab'),
        closable: true,
        popout: true,
        gmOnly: false,
        render: () => <ChatPanel command={command} />,
      },
      {
        kind: 'combat',
        title: t('combat.tab'),
        closable: true,
        popout: true,
        gmOnly: false,
        render: () => <CombatPanel command={command} />,
      },
      {
        kind: 'scenes',
        title: t('scenes.tab'),
        closable: true,
        popout: true,
        gmOnly: true,
        render: () => <ScenesPanel command={command} />,
      },
    ]
    const panels = catalogue.filter((p) => isGm || !p.gmOnly)
    /** defaultLayout is the layout of M1: the canvas, and at its right the group of chat, fight and (the GM's) scenes. */
    const defaultLayout = (api: DockviewApi) => {
      const [sceneDef, ...side] = panels
      if (sceneDef === undefined) return
      const sceneItem = api.addPanel(panelOptions(sceneDef))
      let anchor: IDockviewPanel | undefined
      for (const def of side) {
        const added = api.addPanel({
          ...panelOptions(def),
          inactive: anchor !== undefined,
          ...(anchor === undefined
            ? {
                initialWidth: SIDE_WIDTH_REM * rootFontSize(),
                position: { referencePanel: sceneItem, direction: 'right' as const },
              }
            : { position: { referencePanel: anchor } }),
        })
        anchor ??= added
      }
      api.getPanel('chat')?.api.setActive()
    }
    return (
      <div className="table table--wide">
        {topBar}
        {game === null ? (
          <div className="workspace">
            <ConnectionNote />
          </div>
        ) : (
          <Workspace
            key={layoutKey(accountId, campaignId, 'table')}
            storageKey={layoutKey(accountId, campaignId, 'table')}
            panels={panels}
            defaultLayout={defaultLayout}
            popoutTitle={(panel) =>
              t('workspace.popoutTitle', { panel, campaign: game.campaign.name })
            }
          />
        )}
      </div>
    )
  }

  return (
    <div className="table" data-section={section}>
      {topBar}
      {toolRail}
      <main className="table__stage" inert={section !== 'map'}>
        {stage}
      </main>
      <SidePanel
        tabs={[
          { id: 'chat', label: t('chat.tab'), content: <ChatPanel command={command} /> },
          { id: 'combat', label: t('combat.tab'), content: <CombatPanel command={command} /> },
          ...(isGm
            ? [{ id: 'scenes', label: t('scenes.tab'), content: <ScenesPanel command={command} /> }]
            : []),
        ]}
        selected={panelTab}
        onSelect={(id) => {
          setPanelTab(id)
          setSection(id === 'scenes' || id === 'combat' ? id : 'chat')
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

/** rootFontSize is the size of 1rem in pixels. */
function rootFontSize(): number {
  return parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
}

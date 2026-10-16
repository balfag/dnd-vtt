import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSession } from '../../app/session'
import {
  createCampaign,
  createPasswordReset,
  endGame,
  listCampaigns,
  listPlayers,
  removePlayer,
  renameCampaign,
  rotateLink,
  startGame,
  type Campaign,
  type CampaignPlayer,
  type PasswordResetLink,
} from '../../shared/api/campaigns'
import { ApiError } from '../../shared/api/client'
import { errorKey } from '../../shared/api/errors'
import { gameSocketUrl } from '../../shared/ws/socket'
import { GameConnection } from '../../shared/ws/connection'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Toast, type ToastMessage } from '../../ui/Toast'
import { gameStore, useGame } from '../game/store'
import { ActiveGame } from './ActiveGame'
import { CampaignList } from './CampaignList'
import { mergeParticipants, unknownParticipantIds } from './participants'
import './campaigns.css'

// The game link is shown by the server once, when the game starts or the link
// changes. It lives only here, in the memory of the page: not in a storage, not
// in the address bar, not in the console.
interface RememberedLink {
  gameSessionId: string
  url: string
}

interface PlayersOf {
  campaignId: string
  list: CampaignPlayer[]
}

/** CampaignsPage is the GM's screen: campaigns, the running game and who plays in it. */
export function CampaignsPage({ login }: { login: string }) {
  const { t } = useTranslation()
  const { refresh, logout } = useSession()
  const [campaigns, setCampaigns] = useState<Campaign[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [playersOf, setPlayersOf] = useState<PlayersOf | null>(null)
  const [playersError, setPlayersError] = useState<string | null>(null)
  const [link, setLink] = useState<RememberedLink | null>(null)
  const [toast, setToast] = useState<ToastMessage | null>(null)
  const toastCount = useRef(0)

  // An API error becomes the key of its text; a 401 asks the session to look again,
  // which sends the GM to the sign-in.
  const fail = useCallback(
    (err: unknown): string => {
      if (err instanceof ApiError && err.status === 401) void refresh()
      return errorKey(err)
    },
    [refresh],
  )

  const notify = useCallback((text: string) => {
    toastCount.current += 1
    setToast({ id: toastCount.current, text })
  }, [])
  const dismissToast = useCallback(() => {
    setToast(null)
  }, [])

  // The live part of the screen: who is online comes from the game socket.
  useEffect(() => {
    gameStore.getState().reset()
    const connection = new GameConnection({
      store: gameStore,
      url: gameSocketUrl(window.location),
    })
    connection.start()
    return () => {
      connection.stop()
    }
  }, [])
  const closedReason = useGame((s) =>
    s.connection.state === 'closed' ? s.connection.reason : null,
  )
  useEffect(() => {
    if (closedReason === 'unauthorized') void refresh()
  }, [closedReason, refresh])

  // Data is fetched in effects whose setState calls sit in promise callbacks.
  // A bump of reloadKey fetches the campaigns again (after an action here).
  const [reloadKey, setReloadKey] = useState(0)
  const reload = useCallback(() => {
    setReloadKey((k) => k + 1)
  }, [])
  const [playersKey, setPlayersKey] = useState(0)

  // The game of the store: it also tells about a game started or ended from
  // another device, so a change of its id fetches the campaigns again.
  const liveGame = useGame((s) => s.game)
  const liveGameId = liveGame?.gameSessionId ?? null

  useEffect(() => {
    let current = true
    listCampaigns().then(
      (list) => {
        if (!current) return
        setCampaigns(list)
        setLoadError(null)
      },
      (err: unknown) => {
        if (current) setLoadError(fail(err))
      },
    )
    return () => {
      current = false
    }
  }, [reloadKey, liveGameId, fail])

  const active = campaigns?.find((c) => c.activeGameSession !== null) ?? null
  const activeCampaignId = active?.id ?? null
  const activeGameId = active?.activeGameSession?.id ?? null

  // The live game, when it is the running one.
  const livePlayers = useMemo(
    () =>
      liveGame !== null && activeGameId !== null && liveGame.gameSessionId === activeGameId
        ? liveGame.players
        : null,
    [liveGame, activeGameId],
  )
  const restPlayers = useMemo(
    () => (playersOf !== null && playersOf.campaignId === activeCampaignId ? playersOf.list : []),
    [playersOf, activeCampaignId],
  )

  // Someone who registered during the game is in the store before the REST
  // list: their ids make the key that fetches the list again.
  const unknownKey = useMemo(
    () => (livePlayers === null ? '' : unknownParticipantIds(restPlayers, livePlayers).join(',')),
    [livePlayers, restPlayers],
  )

  useEffect(() => {
    if (activeCampaignId === null) return
    let current = true
    listPlayers(activeCampaignId).then(
      (list) => {
        if (!current) return
        setPlayersOf({ campaignId: activeCampaignId, list })
        setPlayersError(null)
      },
      (err: unknown) => {
        if (current) setPlayersError(fail(err))
      },
    )
    return () => {
      current = false
    }
  }, [activeCampaignId, unknownKey, playersKey, fail])

  async function create(name: string) {
    await createCampaign(name)
    reload()
  }

  async function rename(id: string, name: string) {
    await renameCampaign(id, name)
    reload()
  }

  async function start(c: Campaign) {
    const game = await startGame(c.id)
    setLink({ gameSessionId: game.id, url: game.link })
    // The same campaign can get a new game: its participants are fetched again.
    setPlayersKey((k) => k + 1)
    reload()
    notify(t('game.started'))
  }

  async function rotate() {
    if (activeGameId === null) return
    const { link: url } = await rotateLink(activeGameId)
    setLink({ gameSessionId: activeGameId, url })
    notify(t('game.rotated'))
  }

  async function end() {
    if (activeGameId === null) return
    await endGame(activeGameId)
    setLink(null)
    reload()
    notify(t('game.ended'))
  }

  async function removeParticipant(p: CampaignPlayer) {
    await removePlayer(p.id)
    setPlayersOf((current) =>
      current === null
        ? current
        : {
            ...current,
            list: current.list.map((q) => (q.id === p.id ? { ...q, removed: true } : q)),
          },
    )
  }

  function resetLink(p: CampaignPlayer): Promise<PasswordResetLink> {
    return createPasswordReset(p.accountId)
  }

  const shownLink = link !== null && link.gameSessionId === activeGameId ? link.url : null

  return (
    <main className="campaigns">
      <header className="campaigns__head">
        <h1 className="campaigns__title">{t('campaigns.title')}</h1>
        <div className="campaigns__who">
          <span className="campaigns__login">{login}</span>
          <Button
            variant="secondary"
            fullWidth={false}
            onClick={() => {
              void logout()
            }}
          >
            {t('common.signOut')}
          </Button>
        </div>
      </header>

      {loadError !== null && <ErrorNote>{t(loadError)}</ErrorNote>}
      {playersError !== null && <ErrorNote>{t(playersError)}</ErrorNote>}

      {active !== null && (
        <ActiveGame
          campaign={active}
          link={shownLink}
          players={mergeParticipants(restPlayers, livePlayers)}
          onRotate={rotate}
          onEnd={end}
          onResetLink={resetLink}
          onRemove={removeParticipant}
          fail={fail}
          notify={notify}
        />
      )}

      {campaigns !== null && (
        <CampaignList
          campaigns={campaigns}
          onCreate={create}
          onRename={rename}
          onStart={start}
          fail={fail}
        />
      )}

      <Toast toast={toast} onDone={dismissToast} />
    </main>
  )
}

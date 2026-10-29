import { useStore } from 'zustand'
import { createStore, type StoreApi } from 'zustand/vanilla'
import {
  isChatMessage,
  isCombatChanged,
  isDrawingChanged,
  isDrawingRemoved,
  isFogChanged,
  isPingShown,
  isPresence,
  isRulerShown,
  isSceneActivated,
  isSceneChanged,
  isScenesChanged,
  isSceneViewAck,
  isTokenChanged,
  isTokenDragging,
  isTokenRemoved,
  MessageType,
  type ChatMessage,
  type Combat,
  type Drawing,
  type Envelope,
  type PlayerInfo,
  type SceneListItem,
  type SceneView,
  type Snapshot,
  type SnapshotMe,
  type Token,
} from '../../shared/protocol'

export type CloseReason = 'unauthorized' | 'removed' | 'gameEnded'
export type ConnectionState =
  | { state: 'idle' }
  | { state: 'connecting'; attempt: number } // attempt: 0 for the first connect, n for reconnect n
  | { state: 'synced' }
  | { state: 'reconnecting'; attempt: number; delayMs: number }
  | { state: 'closed'; reason: CloseReason }

export interface GameView {
  gameSessionId: string
  startedAt: number
  campaign: { id: string; name: string }
  players: PlayerInfo[] // order of the snapshot, new players appended
  activeSceneId: string | null
  /** The GM gets every scene; a player gets []. */
  scenes: SceneListItem[]
  /** The scene this client shows; null while there is none. */
  scene: SceneView | null
  /** The scene the GM looks at (id of scene); a player always looks at the active one. */
  viewSceneId: string | null
  /** Messages ascending by time. */
  chat: ChatMessage[]
  /** True when there is no older history to load. */
  chatHistoryDone: boolean
  /** The fight of the shown scene; null: none, or not known yet after the shown scene changed. */
  combat: Combat | null
}

/** A token another client is dragging now: transient, kept outside the snapshot and seq. */
export interface RemoteDrag {
  x: number
  y: number
  /** Date.now() of the last update. */
  at: number
}

/** A remote drag with no update for this long is forgotten. */
export const DRAG_TTL_MS = 2000
/** A ruler of another player that was not refreshed for this long is forgotten. */
export const RULER_TTL_MS = 5000
/** A ping is shown for this long. */
export const PING_TTL_MS = 2000

/** A ruler another player is measuring with now: transient, kept outside the snapshot and seq. */
export interface RemoteRuler {
  points: [number, number][]
  /** Date.now() of the last update. */
  at: number
}

/** A ping on the shown scene. */
export interface PingMark {
  id: string
  playerId: string
  x: number
  y: number
  focus: boolean
  /** Date.now() when it arrived. */
  at: number
}
export interface GameState {
  connection: ConnectionState
  me: SnapshotMe | null
  game: GameView | null
  lastSeq: number
  /** Other clients' drags by token id. */
  dragging: Record<string, RemoteDrag>
  /** Rulers of other players by player id, on the shown scene. */
  rulers: Record<string, RemoteRuler>
  /** Pings of the last 2 s on the shown scene. */
  pings: PingMark[]
  /** The scene the GM looked at before the last snapshot replaced the view; null when there is none to restore. */
  viewBeforeSync: string | null
  /** Handouts to show in a window, the first one now: live events only, not the snapshot or the history. */
  handoutQueue: ChatMessage[]
  setConnection(c: ConnectionState): void
  applySnapshot(s: Snapshot): void
  /** setViewedScene shows a scene the GM asked for with scene.view. */
  setViewedScene(scene: SceneView): void
  /** clearViewBeforeSync forgets the view to restore once it is restored or given up. */
  clearViewBeforeSync(): void
  /** applyEvent applies one event with seq; unknown types only advance lastSeq. */
  applyEvent(e: Envelope & { seq: number }): void
  /** applyTransient applies a message without seq (token.dragging); unknown types are ignored. */
  applyTransient(env: Envelope): void
  /** pruneDragging forgets drags with no update for DRAG_TTL_MS. */
  pruneDragging(now: number): void
  /** pruneTools forgets rulers and pings that have outlived their time. */
  pruneTools(now: number): void
  /** showHandout puts a handout of the chat first in the queue, so its window opens (again). */
  showHandout(message: ChatMessage): void
  /** dismissHandout takes the handout with the id out of the queue. */
  dismissHandout(id: string): void
  /** prependHistory puts older messages (ascending) before the loaded ones, skipping known ids. */
  prependHistory(messages: ChatMessage[]): void
  reset(): void
}

/** The snapshot and a history page carry up to 50 messages: fewer means the history is complete. */
export const historyPageSize = 50

const initial: Pick<
  GameState,
  | 'connection'
  | 'me'
  | 'game'
  | 'lastSeq'
  | 'dragging'
  | 'rulers'
  | 'pings'
  | 'viewBeforeSync'
  | 'handoutQueue'
> = {
  connection: { state: 'idle' },
  me: null,
  game: null,
  lastSeq: 0,
  dragging: {},
  rulers: {},
  pings: [],
  viewBeforeSync: null,
  handoutQueue: [],
}

let pingCount = 0

export function createGameStore(): StoreApi<GameState> {
  return createStore<GameState>()((set) => ({
    ...initial,
    setConnection: (connection) => {
      set({ connection })
    },
    applySnapshot: (s) => {
      set((state) => {
        // A snapshot shows the active scene; the GM's own view comes back after the sync.
        const resumed =
          s.me.isGm && s.game !== null && state.game?.gameSessionId === s.game.gameSessionId
        return {
          me: s.me,
          game:
            s.game === null
              ? null
              : {
                  ...s.game,
                  players: [...s.game.players],
                  scenes: [...s.game.scenes],
                  viewSceneId: s.game.activeSceneId,
                  chat: [...s.game.chat],
                  chatHistoryDone: s.game.chat.length < historyPageSize,
                },
          lastSeq: s.seq,
          viewBeforeSync: resumed ? (state.game?.viewSceneId ?? null) : null,
          dragging: {},
          rulers: {},
          pings: [],
        }
      })
    },
    setViewedScene: (scene) => {
      set((state) => {
        if (state.game === null) return {}
        return {
          game: { ...state.game, scene, viewSceneId: scene.id, combat: null },
          rulers: {},
          pings: [],
        }
      })
    },
    clearViewBeforeSync: () => {
      set({ viewBeforeSync: null })
    },
    applyEvent: (e) => {
      set((state) => {
        const game = state.game
        if (game === null) return { lastSeq: e.seq }
        if (isSceneActivated(e)) {
          const next = e.d.scene
          // A GM who prepares a third scene keeps it on the canvas.
          const follows =
            state.me?.isGm !== true ||
            game.viewSceneId === game.activeSceneId ||
            game.viewSceneId === next.id
          return {
            lastSeq: e.seq,
            game: follows
              ? {
                  ...game,
                  scene: next,
                  activeSceneId: next.id,
                  viewSceneId: next.id,
                  combat: null,
                }
              : { ...game, activeSceneId: next.id },
            dragging: {},
            rulers: {},
            pings: [],
          }
        }
        if (isFogChanged(e)) {
          if (game.scene?.id !== e.d.sceneId) return { lastSeq: e.seq }
          const scene = { ...game.scene, fog: e.d.fog }
          if (e.d.tokens !== undefined) scene.tokens = e.d.tokens
          return { lastSeq: e.seq, game: { ...game, scene } }
        }
        if (isTokenChanged(e)) {
          const token = e.d.token
          const dragging = withoutDrag(state.dragging, token.id)
          if (game.scene?.id !== token.sceneId) return { lastSeq: e.seq, dragging }
          return {
            lastSeq: e.seq,
            dragging,
            game: {
              ...game,
              scene: { ...game.scene, tokens: upsertToken(game.scene.tokens, token) },
            },
          }
        }
        if (isTokenRemoved(e)) {
          const id = e.d.tokenId
          const dragging = withoutDrag(state.dragging, id)
          if (game.scene === null || !game.scene.tokens.some((t) => t.id === id)) {
            return { lastSeq: e.seq, dragging }
          }
          const tokens = game.scene.tokens.filter((t) => t.id !== id)
          return { lastSeq: e.seq, dragging, game: { ...game, scene: { ...game.scene, tokens } } }
        }
        if (isDrawingChanged(e)) {
          const drawing = e.d.drawing
          if (game.scene?.id !== drawing.sceneId) return { lastSeq: e.seq }
          const drawings = upsertDrawing(game.scene.drawings, drawing)
          return { lastSeq: e.seq, game: { ...game, scene: { ...game.scene, drawings } } }
        }
        if (isDrawingRemoved(e)) {
          const id = e.d.drawingId
          if (game.scene === null || !game.scene.drawings.some((d) => d.id === id)) {
            return { lastSeq: e.seq }
          }
          const drawings = game.scene.drawings.filter((d) => d.id !== id)
          return { lastSeq: e.seq, game: { ...game, scene: { ...game.scene, drawings } } }
        }
        if (isSceneChanged(e)) {
          // scene.changed has no tokens: merge, so the client keeps its own.
          if (game.scene?.id !== e.d.scene.id) return { lastSeq: e.seq }
          return { lastSeq: e.seq, game: { ...game, scene: { ...game.scene, ...e.d.scene } } }
        }
        if (isScenesChanged(e)) {
          const scenes = e.d.scenes
          const gone =
            state.me?.isGm === true &&
            game.viewSceneId !== null &&
            !scenes.some((x) => x.id === game.viewSceneId)
          // The viewed scene was deleted: the canvas is empty until the active one is asked for.
          return {
            lastSeq: e.seq,
            game: gone ? { ...game, scenes, scene: null, viewSceneId: null } : { ...game, scenes },
          }
        }
        if (isCombatChanged(e)) {
          // The GM may look at another scene: its fight is not the one on the screen.
          if (game.scene?.id !== e.d.sceneId) return { lastSeq: e.seq }
          return { lastSeq: e.seq, game: { ...game, combat: e.d.combat } }
        }
        if (isChatMessage(e)) {
          if (game.chat.some((m) => m.id === e.d.id)) return { lastSeq: e.seq }
          const chat = { ...game, chat: [...game.chat, e.d] }
          // The GM sent it: the window is for the players.
          if (e.d.kind !== 'handout' || state.me?.isGm === true)
            return { lastSeq: e.seq, game: chat }
          return { lastSeq: e.seq, game: chat, handoutQueue: [...state.handoutQueue, e.d] }
        }
        if (!isPresence(e)) return { lastSeq: e.seq }
        const { playerId, online, player } = e.d
        if (game.players.some((p) => p.id === playerId)) {
          const players = game.players.map((p) =>
            p.id === playerId ? { ...p, ...player, online } : p,
          )
          return { lastSeq: e.seq, game: { ...game, players } }
        }
        if (player === undefined) return { lastSeq: e.seq }
        return {
          lastSeq: e.seq,
          game: { ...game, players: [...game.players, { ...player, online }] },
        }
      })
    },
    applyTransient: (env) => {
      const now = Date.now()
      if (isTokenDragging(env)) {
        set((state) => ({
          dragging: {
            ...liveDrags(state.dragging, now),
            [env.d.tokenId]: { x: env.d.x, y: env.d.y, at: now },
          },
        }))
      } else if (isRulerShown(env)) {
        const { playerId, sceneId, points } = env.d
        set((state) => {
          if (state.game?.scene?.id !== sceneId) return {}
          const rest = Object.fromEntries(
            Object.entries(liveRulers(state.rulers, now)).filter(([id]) => id !== playerId),
          )
          return {
            rulers: points.length === 0 ? rest : { ...rest, [playerId]: { points, at: now } },
          }
        })
      } else if (isPingShown(env)) {
        const { playerId, sceneId, x, y, focus } = env.d
        set((state) => {
          if (state.game?.scene?.id !== sceneId) return {}
          pingCount++
          const mark = { id: `ping-${String(pingCount)}`, playerId, x, y, focus, at: now }
          return { pings: [...livePings(state.pings, now), mark] }
        })
      }
    },
    pruneDragging: (now) => {
      set((state) => {
        const live = liveDrags(state.dragging, now)
        return Object.keys(live).length === Object.keys(state.dragging).length
          ? state
          : { dragging: live }
      })
    },
    pruneTools: (now) => {
      set((state) => {
        const rulers = liveRulers(state.rulers, now)
        const pings = livePings(state.pings, now)
        const sameRulers = Object.keys(rulers).length === Object.keys(state.rulers).length
        return sameRulers && pings.length === state.pings.length ? state : { rulers, pings }
      })
    },
    showHandout: (message) => {
      set((state) => ({
        handoutQueue: [message, ...state.handoutQueue.filter((m) => m.id !== message.id)],
      }))
    },
    dismissHandout: (id) => {
      set((state) => ({ handoutQueue: state.handoutQueue.filter((m) => m.id !== id) }))
    },
    prependHistory: (messages) => {
      set((state) => {
        const game = state.game
        if (game === null) return {}
        const known = new Set(game.chat.map((m) => m.id))
        const older = messages.filter((m) => {
          if (known.has(m.id)) return false
          known.add(m.id)
          return true
        })
        // A short page is the oldest one.
        const chatHistoryDone = game.chatHistoryDone || messages.length < historyPageSize
        return { game: { ...game, chat: [...older, ...game.chat], chatHistoryDone } }
      })
    },
    reset: () => {
      set({ ...initial })
    },
  }))
}

function withoutDrag(
  dragging: Record<string, RemoteDrag>,
  tokenId: string,
): Record<string, RemoteDrag> {
  if (!(tokenId in dragging)) return dragging
  return Object.fromEntries(Object.entries(dragging).filter(([id]) => id !== tokenId))
}

function liveDrags(dragging: Record<string, RemoteDrag>, now: number): Record<string, RemoteDrag> {
  return Object.fromEntries(Object.entries(dragging).filter(([, d]) => now - d.at < DRAG_TTL_MS))
}

function liveRulers(rulers: Record<string, RemoteRuler>, now: number): Record<string, RemoteRuler> {
  return Object.fromEntries(Object.entries(rulers).filter(([, r]) => now - r.at < RULER_TTL_MS))
}

function livePings(pings: PingMark[], now: number): PingMark[] {
  return pings.filter((p) => now - p.at < PING_TTL_MS)
}

function upsertDrawing(drawings: Drawing[], drawing: Drawing): Drawing[] {
  return drawings.some((d) => d.id === drawing.id)
    ? drawings.map((d) => (d.id === drawing.id ? drawing : d))
    : [...drawings, drawing]
}

function upsertToken(tokens: Token[], token: Token): Token[] {
  return tokens.some((t) => t.id === token.id)
    ? tokens.map((t) => (t.id === token.id ? token : t))
    : [...tokens, token]
}

export const gameStore: StoreApi<GameState> = createGameStore()

/** viewScene asks the server for a scene (scene.view) and shows it; only the GM may. */
export async function viewScene(
  command: (type: string, data?: unknown) => Promise<unknown>,
  sceneId: string,
): Promise<void> {
  const d = await command(MessageType.SceneView, { sceneId })
  const ack = { t: MessageType.Ack, re: '', d }
  if (isSceneViewAck(ack)) gameStore.getState().setViewedScene(ack.d.scene)
}

export function useGame<T>(selector: (s: GameState) => T): T {
  return useStore(gameStore, selector)
}

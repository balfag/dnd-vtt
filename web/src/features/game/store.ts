import { useStore } from 'zustand'
import { createStore, type StoreApi } from 'zustand/vanilla'
import {
  isChatMessage,
  isFogChanged,
  isPresence,
  isSceneActivated,
  isSceneChanged,
  isScenesChanged,
  isSceneViewAck,
  isTokenChanged,
  isTokenDragging,
  isTokenRemoved,
  MessageType,
  type ChatMessage,
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

export interface GameState {
  connection: ConnectionState
  me: SnapshotMe | null
  game: GameView | null
  lastSeq: number
  /** Other clients' drags by token id. */
  dragging: Record<string, RemoteDrag>
  /** The scene the GM looked at before the last snapshot replaced the view; null when there is none to restore. */
  viewBeforeSync: string | null
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
  /** prependHistory puts older messages (ascending) before the loaded ones, skipping known ids. */
  prependHistory(messages: ChatMessage[]): void
  reset(): void
}

/** The snapshot and a history page carry up to 50 messages: fewer means the history is complete. */
export const historyPageSize = 50

const initial: Pick<
  GameState,
  'connection' | 'me' | 'game' | 'lastSeq' | 'dragging' | 'viewBeforeSync'
> = {
  connection: { state: 'idle' },
  me: null,
  game: null,
  lastSeq: 0,
  dragging: {},
  viewBeforeSync: null,
}

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
        }
      })
    },
    setViewedScene: (scene) => {
      set((state) => {
        if (state.game === null) return {}
        return { game: { ...state.game, scene, viewSceneId: scene.id } }
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
              ? { ...game, scene: next, activeSceneId: next.id, viewSceneId: next.id }
              : { ...game, activeSceneId: next.id },
            dragging: {},
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
        if (isChatMessage(e)) {
          if (game.chat.some((m) => m.id === e.d.id)) return { lastSeq: e.seq }
          return { lastSeq: e.seq, game: { ...game, chat: [...game.chat, e.d] } }
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
      if (!isTokenDragging(env)) return
      const now = Date.now()
      set((state) => ({
        dragging: {
          ...liveDrags(state.dragging, now),
          [env.d.tokenId]: { x: env.d.x, y: env.d.y, at: now },
        },
      }))
    },
    pruneDragging: (now) => {
      set((state) => {
        const live = liveDrags(state.dragging, now)
        return Object.keys(live).length === Object.keys(state.dragging).length
          ? state
          : { dragging: live }
      })
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

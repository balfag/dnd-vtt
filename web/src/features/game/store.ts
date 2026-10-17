import { useStore } from 'zustand'
import { createStore, type StoreApi } from 'zustand/vanilla'
import {
  isFogChanged,
  isPresence,
  isSceneActivated,
  isSceneChanged,
  isScenesChanged,
  isTokenChanged,
  isTokenDragging,
  isTokenRemoved,
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
  setConnection(c: ConnectionState): void
  applySnapshot(s: Snapshot): void
  /** applyEvent applies one event with seq; unknown types only advance lastSeq. */
  applyEvent(e: Envelope & { seq: number }): void
  /** applyTransient applies a message without seq (token.dragging); unknown types are ignored. */
  applyTransient(env: Envelope): void
  /** pruneDragging forgets drags with no update for DRAG_TTL_MS. */
  pruneDragging(now: number): void
  reset(): void
}

const initial: Pick<GameState, 'connection' | 'me' | 'game' | 'lastSeq' | 'dragging'> = {
  connection: { state: 'idle' },
  me: null,
  game: null,
  lastSeq: 0,
  dragging: {},
}

export function createGameStore(): StoreApi<GameState> {
  return createStore<GameState>()((set) => ({
    ...initial,
    setConnection: (connection) => {
      set({ connection })
    },
    applySnapshot: (s) => {
      set({
        me: s.me,
        game:
          s.game === null
            ? null
            : {
                ...s.game,
                players: [...s.game.players],
                scenes: [...s.game.scenes],
              },
        lastSeq: s.seq,
        dragging: {},
      })
    },
    applyEvent: (e) => {
      set((state) => {
        const game = state.game
        if (game === null) return { lastSeq: e.seq }
        if (isSceneActivated(e)) {
          return {
            lastSeq: e.seq,
            game: { ...game, scene: e.d.scene, activeSceneId: e.d.scene.id },
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
          return { lastSeq: e.seq, game: { ...game, scenes: e.d.scenes } }
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

export function useGame<T>(selector: (s: GameState) => T): T {
  return useStore(gameStore, selector)
}

import { useStore } from 'zustand'
import { createStore, type StoreApi } from 'zustand/vanilla'
import {
  isPresence,
  isSceneActivated,
  isSceneChanged,
  isScenesChanged,
  type Envelope,
  type PlayerInfo,
  type SceneListItem,
  type SceneView,
  type Snapshot,
  type SnapshotMe,
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

export interface GameState {
  connection: ConnectionState
  me: SnapshotMe | null
  game: GameView | null
  lastSeq: number
  setConnection(c: ConnectionState): void
  applySnapshot(s: Snapshot): void
  /** applyEvent applies one event with seq; unknown types only advance lastSeq. */
  applyEvent(e: Envelope & { seq: number }): void
  reset(): void
}

const initial: Pick<GameState, 'connection' | 'me' | 'game' | 'lastSeq'> = {
  connection: { state: 'idle' },
  me: null,
  game: null,
  lastSeq: 0,
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
          }
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
    reset: () => {
      set({ ...initial })
    },
  }))
}

export const gameStore: StoreApi<GameState> = createGameStore()

export function useGame<T>(selector: (s: GameState) => T): T {
  return useStore(gameStore, selector)
}

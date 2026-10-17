import { describe, expect, it } from 'vitest'
import type {
  PlayerInfo,
  SceneInfo,
  SceneListItem,
  SceneView,
  Snapshot,
  Token,
} from '../../shared/protocol'
import { createGameStore, type GameState } from './store'
import type { StoreApi } from 'zustand/vanilla'

const gm: PlayerInfo = { id: 'p1', displayName: 'GM', color: '#aa0000', role: 'gm', online: true }
const ann: PlayerInfo = {
  id: 'p2',
  displayName: 'Ann',
  color: '#00aa00',
  role: 'player',
  online: false,
}
const bob: PlayerInfo = {
  id: 'p3',
  displayName: 'Bob',
  color: '#0000aa',
  role: 'player',
  online: true,
}

function snapshot(seq = 4): Snapshot {
  return {
    seq,
    me: { accountId: 'a1', isGm: false, playerId: 'p2' },
    game: {
      gameSessionId: 'g1',
      startedAt: 1000,
      campaign: { id: 'c1', name: 'Campaign' },
      players: [gm, ann],
      activeSceneId: null,
      scenes: [],
      scene: null,
      chat: [],
    },
  }
}

function players(store: StoreApi<GameState>): PlayerInfo[] {
  const game = store.getState().game
  if (game === null) throw new Error('no game')
  return game.players
}

describe('game store', () => {
  it('applies a snapshot', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    const s = store.getState()
    expect(players(store)).toEqual([gm, ann])
    expect(s.game?.gameSessionId).toBe('g1')
    expect(s.me?.playerId).toBe('p2')
    expect(s.lastSeq).toBe(4)
  })

  it('applies a snapshot without a game', () => {
    const store = createGameStore()
    store
      .getState()
      .applySnapshot({ seq: 0, me: { accountId: 'a1', isGm: true, playerId: null }, game: null })
    expect(store.getState().game).toBeNull()
    expect(store.getState().me?.isGm).toBe(true)
  })

  it('changes only online for a known player', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    const before = players(store)
    store
      .getState()
      .applyEvent({ t: 'presence.update', seq: 5, d: { playerId: 'p2', online: true } })
    const after = players(store)
    expect(after[1]).toEqual({ ...ann, online: true })
    expect(after[0]).toBe(before[0])
    expect(after).not.toBe(before)
    expect(store.getState().lastSeq).toBe(5)
  })

  it('appends a new player with online from the event', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    store.getState().applyEvent({
      t: 'presence.update',
      seq: 5,
      d: { playerId: 'p3', online: false, player: bob },
    })
    const got = players(store)
    expect(got).toHaveLength(3)
    expect(got[2]).toEqual({ ...bob, online: false })
  })

  it('updates fields of a known player but takes online from the event', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    store.getState().applyEvent({
      t: 'presence.update',
      seq: 5,
      d: { playerId: 'p2', online: true, player: { ...ann, displayName: 'Anna', online: false } },
    })
    expect(players(store)[1]).toEqual({ ...ann, displayName: 'Anna', online: true })
  })

  it('ignores an unknown player without player data', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    const before = store.getState().game
    store
      .getState()
      .applyEvent({ t: 'presence.update', seq: 5, d: { playerId: 'zz', online: true } })
    expect(store.getState().game).toBe(before)
    expect(store.getState().lastSeq).toBe(5)
  })

  it('only advances lastSeq for an unknown event type', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    const before = store.getState().game
    store.getState().applyEvent({ t: 'scene.changed', seq: 9, d: { x: 1 } })
    expect(store.getState().lastSeq).toBe(9)
    expect(store.getState().game).toBe(before)
  })

  it('only advances lastSeq for a malformed presence', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    const before = store.getState().game
    store.getState().applyEvent({ t: 'presence.update', seq: 6, d: { playerId: 5 } })
    expect(store.getState().lastSeq).toBe(6)
    expect(store.getState().game).toBe(before)
  })

  it('only advances lastSeq when there is no game', () => {
    const store = createGameStore()
    store.getState().applyEvent({
      t: 'presence.update',
      seq: 2,
      d: { playerId: 'p2', online: true, player: ann },
    })
    expect(store.getState().game).toBeNull()
    expect(store.getState().lastSeq).toBe(2)
  })

  describe('scenes', () => {
    const token: Token = {
      id: 't1',
      sceneId: 's1',
      layer: 'tokens',
      x: 10,
      y: 20,
      w: 1,
      h: 1,
      rotation: 0,
      z: 0,
      name: 'Orc',
      nameVisibility: 'all',
      image: null,
      hidden: false,
      locked: false,
      owners: [],
      bars: [],
      conditions: [],
    }
    const scene: SceneView = {
      id: 's1',
      name: 'Tavern',
      width: 3000,
      height: 2000,
      grid: {
        type: 'square',
        sizePx: 70,
        unitFt: 5,
        offsetX: 0,
        offsetY: 0,
        color: '#9aa8b3',
        visible: true,
      },
      background: null,
      fog: { enabled: false, ops: [] },
      tokens: [token],
    }
    const list: SceneListItem[] = [
      { id: 's1', name: 'Tavern', sort: 0 },
      { id: 's2', name: 'Crypt', sort: 1 },
    ]

    function infoOf(s: SceneView): SceneInfo {
      return {
        id: s.id,
        name: s.name,
        width: s.width,
        height: s.height,
        grid: s.grid,
        background: s.background,
        fog: s.fog,
      }
    }

    function withScene(): StoreApi<GameState> {
      const store = createGameStore()
      const snap = snapshot()
      if (snap.game === null) throw new Error('no game')
      snap.game = { ...snap.game, activeSceneId: 's1', scenes: list, scene }
      store.getState().applySnapshot(snap)
      return store
    }

    function sceneOf(store: StoreApi<GameState>): SceneView | null {
      return store.getState().game?.scene ?? null
    }

    it('takes the scenes from the snapshot', () => {
      const store = withScene()
      const game = store.getState().game
      expect(game?.activeSceneId).toBe('s1')
      expect(game?.scenes).toEqual(list)
      expect(game?.scene).toEqual(scene)
    })

    it('scene.activated replaces the scene and the active id', () => {
      const store = withScene()
      const next: SceneView = { ...scene, id: 's2', name: 'Crypt', tokens: [] }
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: next } })
      expect(sceneOf(store)).toEqual(next)
      expect(store.getState().game?.activeSceneId).toBe('s2')
      expect(store.getState().lastSeq).toBe(5)
    })

    it('scene.activated sets a scene when there was none', () => {
      const store = createGameStore()
      store.getState().applySnapshot(snapshot())
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene } })
      expect(sceneOf(store)?.id).toBe('s1')
      expect(store.getState().game?.activeSceneId).toBe('s1')
    })

    it('scene.changed for another scene changes nothing', () => {
      const store = withScene()
      const before = store.getState().game
      const info = infoOf(scene)
      store
        .getState()
        .applyEvent({ t: 'scene.changed', seq: 5, d: { scene: { ...info, id: 's2', name: 'X' } } })
      expect(store.getState().game).toBe(before)
      expect(store.getState().lastSeq).toBe(5)
    })

    it('scene.changed for the current scene updates fields and keeps the tokens', () => {
      const store = withScene()
      const info = infoOf(scene)
      store.getState().applyEvent({
        t: 'scene.changed',
        seq: 5,
        d: {
          scene: {
            ...info,
            name: 'Tavern 2',
            width: 4000,
            background: { assetId: 'a1', url: '/assets/a1' },
          },
        },
      })
      const got = sceneOf(store)
      expect(got?.name).toBe('Tavern 2')
      expect(got?.width).toBe(4000)
      expect(got?.background).toEqual({ assetId: 'a1', url: '/assets/a1' })
      expect(got?.tokens).toEqual([token])
    })

    it('scenes.changed replaces the list', () => {
      const store = withScene()
      const next = [{ id: 's2', name: 'Crypt', sort: 0 }]
      store.getState().applyEvent({ t: 'scenes.changed', seq: 5, d: { scenes: next } })
      expect(store.getState().game?.scenes).toEqual(next)
      expect(sceneOf(store)).toEqual(scene)
    })

    it('a malformed scene event only advances lastSeq', () => {
      const store = withScene()
      const before = store.getState().game
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: { id: 's9' } } })
      store.getState().applyEvent({ t: 'scenes.changed', seq: 6, d: { scenes: 'x' } })
      expect(store.getState().game).toBe(before)
      expect(store.getState().lastSeq).toBe(6)
    })
  })

  it('resets to the initial state', () => {
    const store = createGameStore()
    store.getState().applySnapshot(snapshot())
    store.getState().setConnection({ state: 'synced' })
    store.getState().reset()
    const s = store.getState()
    expect(s.connection).toEqual({ state: 'idle' })
    expect(s.me).toBeNull()
    expect(s.game).toBeNull()
    expect(s.lastSeq).toBe(0)
  })
})

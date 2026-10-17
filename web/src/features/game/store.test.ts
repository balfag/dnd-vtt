import { afterEach, describe, expect, it, vi } from 'vitest'
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

    describe('tokens', () => {
      const dragging = (x: number, y: number) => ({
        t: 'token.dragging',
        d: { tokenId: 't1', x, y },
      })
      const changed = (seq: number, tk: Token) => ({ t: 'token.changed', seq, d: { token: tk } })
      const tokensOf = (store: StoreApi<GameState>) => sceneOf(store)?.tokens ?? []

      afterEach(() => {
        vi.useRealTimers()
      })

      it('inserts a token of the current scene', () => {
        const store = withScene()
        const orc2 = { ...token, id: 't2', name: 'Orc 2' }
        store.getState().applyEvent(changed(5, orc2))
        expect(tokensOf(store)).toEqual([token, orc2])
        expect(store.getState().lastSeq).toBe(5)
      })

      it('replaces a token by id', () => {
        const store = withScene()
        const moved = { ...token, x: 99 }
        store.getState().applyEvent(changed(5, moved))
        expect(tokensOf(store)).toEqual([moved])
      })

      it('ignores a token of another scene', () => {
        const store = withScene()
        const before = store.getState().game
        store.getState().applyEvent(changed(5, { ...token, id: 't9', sceneId: 's2' }))
        expect(store.getState().game).toBe(before)
        expect(store.getState().lastSeq).toBe(5)
      })

      it('removes a token', () => {
        const store = withScene()
        store.getState().applyEvent({ t: 'token.removed', seq: 5, d: { tokenId: 't1' } })
        expect(tokensOf(store)).toEqual([])
      })

      it('ignores the removal of an unknown token', () => {
        const store = withScene()
        const before = store.getState().game
        store.getState().applyEvent({ t: 'token.removed', seq: 5, d: { tokenId: 'zz' } })
        expect(store.getState().game).toBe(before)
      })

      it('fog.changed with tokens replaces the list', () => {
        const store = withScene()
        const fog = { enabled: true, ops: [] }
        const other = { ...token, id: 't7' }
        store
          .getState()
          .applyEvent({ t: 'fog.changed', seq: 5, d: { sceneId: 's1', fog, tokens: [other] } })
        expect(tokensOf(store)).toEqual([other])
        expect(sceneOf(store)?.fog).toEqual(fog)
      })

      it('fog.changed without tokens keeps the list', () => {
        const store = withScene()
        const fog = { enabled: true, ops: [] }
        store.getState().applyEvent({ t: 'fog.changed', seq: 5, d: { sceneId: 's1', fog } })
        expect(tokensOf(store)).toEqual([token])
        expect(sceneOf(store)?.fog).toEqual(fog)
      })

      it('fog.changed for another scene changes nothing', () => {
        const store = withScene()
        const before = store.getState().game
        store.getState().applyEvent({
          t: 'fog.changed',
          seq: 5,
          d: { sceneId: 's2', fog: { enabled: true, ops: [] }, tokens: [] },
        })
        expect(store.getState().game).toBe(before)
      })

      it('records the drag of another client outside lastSeq', () => {
        vi.useFakeTimers()
        vi.setSystemTime(10_000)
        const store = withScene()
        store.getState().applyTransient(dragging(30, 40))
        expect(store.getState().dragging).toEqual({ t1: { x: 30, y: 40, at: 10_000 } })
        expect(store.getState().lastSeq).toBe(4)
      })

      it('forgets the drag when the token changes or goes', () => {
        const store = withScene()
        store.getState().applyTransient(dragging(30, 40))
        store.getState().applyEvent(changed(5, { ...token, x: 30, y: 40 }))
        expect(store.getState().dragging).toEqual({})
        store.getState().applyTransient(dragging(31, 41))
        store.getState().applyEvent({ t: 'token.removed', seq: 6, d: { tokenId: 't1' } })
        expect(store.getState().dragging).toEqual({})
      })

      it('forgets a drag after 2 s without updates', () => {
        vi.useFakeTimers()
        vi.setSystemTime(10_000)
        const store = withScene()
        store.getState().applyTransient(dragging(30, 40))
        store.getState().pruneDragging(11_999)
        expect(Object.keys(store.getState().dragging)).toEqual(['t1'])
        store.getState().pruneDragging(12_000)
        expect(store.getState().dragging).toEqual({})
      })

      it('drops stale drags when another transient arrives', () => {
        vi.useFakeTimers()
        vi.setSystemTime(10_000)
        const store = withScene()
        store.getState().applyTransient(dragging(30, 40))
        vi.setSystemTime(12_500)
        store.getState().applyTransient({ t: 'token.dragging', d: { tokenId: 't2', x: 1, y: 2 } })
        expect(Object.keys(store.getState().dragging)).toEqual(['t2'])
      })

      it('ignores a transient it does not know', () => {
        const store = withScene()
        store.getState().applyTransient({ t: 'ping.shown', d: {} })
        store.getState().applyTransient({ t: 'token.dragging', d: { tokenId: 1 } })
        expect(store.getState().dragging).toEqual({})
      })

      it('scene.activated forgets drags', () => {
        const store = withScene()
        store.getState().applyTransient(dragging(30, 40))
        store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene } })
        expect(store.getState().dragging).toEqual({})
      })
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

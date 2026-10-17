import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  ChatMessage,
  Combat,
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
      combat: null,
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
      drawings: [],
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

    function asGm(store: StoreApi<GameState>, activeId: string | null = 's1') {
      const game = store.getState().game
      if (game === null) throw new Error('no game')
      const snap: Snapshot = {
        seq: store.getState().lastSeq,
        me: { accountId: 'a1', isGm: true, playerId: 'p1' },
        game: {
          ...game,
          activeSceneId: activeId,
          combat: null,
          scenes: [...list, { id: 's3', name: 'Cellar', sort: 2 }],
        },
      }
      store.getState().applySnapshot(snap)
    }
    const crypt: SceneView = { ...scene, id: 's2', name: 'Crypt', tokens: [] }
    const cellar: SceneView = { ...scene, id: 's3', name: 'Cellar', tokens: [] }

    it('a snapshot makes the view the active scene', () => {
      const store = withScene()
      expect(store.getState().game?.viewSceneId).toBe('s1')
      const bare = createGameStore()
      bare.getState().applySnapshot(snapshot())
      expect(bare.getState().game?.viewSceneId).toBeNull()
    })

    it('setViewedScene shows the scene and remembers its id', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene(crypt)
      expect(sceneOf(store)).toEqual(crypt)
      expect(store.getState().game?.viewSceneId).toBe('s2')
      expect(store.getState().game?.activeSceneId).toBe('s1')
    })

    it('a GM who looked at the old active scene follows the activation', () => {
      const store = withScene()
      asGm(store)
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: crypt } })
      expect(sceneOf(store)?.id).toBe('s2')
      expect(store.getState().game?.viewSceneId).toBe('s2')
      expect(store.getState().game?.activeSceneId).toBe('s2')
    })

    it('a GM who looked at the scene being activated gets it refreshed', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene({ ...crypt, name: 'old' })
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: crypt } })
      expect(sceneOf(store)).toEqual(crypt)
      expect(store.getState().game?.viewSceneId).toBe('s2')
      expect(store.getState().game?.activeSceneId).toBe('s2')
    })

    it('a GM who prepares a third scene keeps it on the canvas', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene(cellar)
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: crypt } })
      expect(sceneOf(store)).toEqual(cellar)
      expect(store.getState().game?.viewSceneId).toBe('s3')
      expect(store.getState().game?.activeSceneId).toBe('s2')
    })

    it('a player always follows the activation', () => {
      const store = withScene()
      store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: crypt } })
      expect(sceneOf(store)).toEqual(crypt)
      expect(store.getState().game?.viewSceneId).toBe('s2')
    })

    it('scenes.changed without the viewed scene empties the canvas', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene(cellar)
      const next = [{ id: 's1', name: 'Tavern', sort: 0 }]
      store.getState().applyEvent({ t: 'scenes.changed', seq: 5, d: { scenes: next } })
      expect(sceneOf(store)).toBeNull()
      expect(store.getState().game?.viewSceneId).toBeNull()
      expect(store.getState().game?.activeSceneId).toBe('s1')
    })

    it('scenes.changed keeps the viewed scene that stays in the list', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene(cellar)
      const next = [{ id: 's3', name: 'Cellar', sort: 0 }, ...list]
      store.getState().applyEvent({ t: 'scenes.changed', seq: 5, d: { scenes: next } })
      expect(sceneOf(store)).toEqual(cellar)
    })

    it('a GM snapshot of the same game remembers the view from before it', () => {
      const store = withScene()
      asGm(store)
      store.getState().setViewedScene(cellar)
      asGm(store)
      expect(store.getState().viewBeforeSync).toBe('s3')
      expect(store.getState().game?.viewSceneId).toBe('s1')
      store.getState().clearViewBeforeSync()
      expect(store.getState().viewBeforeSync).toBeNull()
    })

    it('a player snapshot remembers no view', () => {
      const store = withScene()
      store.getState().applySnapshot(snapshot())
      expect(store.getState().viewBeforeSync).toBeNull()
    })

    describe('combat', () => {
      const fight: Combat = {
        sceneId: 's1',
        round: 2,
        turnCombatantId: 'c1',
        combatants: [
          { id: 'c1', tokenId: 't1', name: 'Orc', initiative: 14, hidden: false },
          { id: 'c2', tokenId: 't2', name: '', initiative: null, hidden: true },
        ],
      }

      function withFight(): StoreApi<GameState> {
        const store = createGameStore()
        const snap = snapshot()
        if (snap.game === null) throw new Error('no game')
        snap.game = { ...snap.game, activeSceneId: 's1', scenes: list, scene, combat: fight }
        store.getState().applySnapshot(snap)
        return store
      }

      function combatOf(store: StoreApi<GameState>): Combat | null | undefined {
        return store.getState().game?.combat
      }

      it('takes the fight from the snapshot', () => {
        expect(combatOf(withFight())).toEqual(fight)
      })

      it('combat.changed replaces the fight of the shown scene', () => {
        const store = withFight()
        const next: Combat = { ...fight, round: 3, turnCombatantId: 'c2' }
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: next } })
        expect(combatOf(store)).toEqual(next)
        expect(store.getState().lastSeq).toBe(5)
      })

      it('combat.changed starts a fight where there was none', () => {
        const store = withScene()
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: fight } })
        expect(combatOf(store)).toEqual(fight)
      })

      it('combat.changed of another scene changes nothing', () => {
        const store = withFight()
        const before = store.getState().game
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's2', combat: null } })
        expect(store.getState().game).toBe(before)
        expect(store.getState().lastSeq).toBe(5)
      })

      it('combat.changed with null ends the fight', () => {
        const store = withFight()
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: null } })
        expect(combatOf(store)).toBeNull()
      })

      it('a malformed combat.changed only advances lastSeq', () => {
        const store = withFight()
        const before = store.getState().game
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: { round: 1 } } })
        expect(store.getState().game).toBe(before)
        expect(store.getState().lastSeq).toBe(5)
      })

      it('scene.activated drops the fight until the next combat.changed', () => {
        const store = withFight()
        store.getState().applyEvent({ t: 'scene.activated', seq: 5, d: { scene: crypt } })
        expect(combatOf(store)).toBeNull()
        const next: Combat = { ...fight, sceneId: 's2' }
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 6, d: { sceneId: 's2', combat: next } })
        expect(combatOf(store)).toEqual(next)
      })

      it('setViewedScene drops the fight', () => {
        const store = withFight()
        asGm(store)
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: fight } })
        expect(combatOf(store)).toEqual(fight)
        store.getState().setViewedScene(crypt)
        expect(combatOf(store)).toBeNull()
      })

      it('a GM who prepares another scene keeps no fight and ignores the active one', () => {
        const store = withScene()
        asGm(store)
        store.getState().setViewedScene(cellar)
        store
          .getState()
          .applyEvent({ t: 'combat.changed', seq: 5, d: { sceneId: 's1', combat: fight } })
        expect(combatOf(store)).toBeNull()
      })
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

  describe('chat', () => {
    function msg(n: number, text = `m${String(n)}`): ChatMessage {
      return {
        id: `m${String(n)}`,
        createdAt: 1000 + n,
        authorPlayerId: 'p2',
        kind: 'text',
        visibility: 'public',
        text,
      }
    }
    function range(from: number, to: number): ChatMessage[] {
      const out: ChatMessage[] = []
      for (let n = from; n <= to; n++) out.push(msg(n))
      return out
    }
    function withChat(chat: ChatMessage[]): StoreApi<GameState> {
      const store = createGameStore()
      const snap = snapshot()
      if (snap.game === null) throw new Error('no game')
      snap.game = { ...snap.game, chat }
      store.getState().applySnapshot(snap)
      return store
    }
    function ids(store: StoreApi<GameState>): string[] {
      return (store.getState().game?.chat ?? []).map((m) => m.id)
    }

    it('takes the chat from the snapshot', () => {
      const store = withChat(range(1, 3))
      expect(ids(store)).toEqual(['m1', 'm2', 'm3'])
    })

    it('chat.message appends', () => {
      const store = withChat(range(1, 2))
      store.getState().applyEvent({ t: 'chat.message', seq: 5, d: msg(3) })
      expect(ids(store)).toEqual(['m1', 'm2', 'm3'])
      expect(store.getState().lastSeq).toBe(5)
    })

    it('chat.message with a known id is not added twice', () => {
      const store = withChat(range(1, 2))
      store.getState().applyEvent({ t: 'chat.message', seq: 5, d: msg(2, 'again') })
      expect(ids(store)).toEqual(['m1', 'm2'])
      expect(store.getState().game?.chat[1]?.text).toBe('m2')
      expect(store.getState().lastSeq).toBe(5)
    })

    it('a malformed chat.message only advances lastSeq', () => {
      const store = withChat(range(1, 2))
      store.getState().applyEvent({ t: 'chat.message', seq: 5, d: { id: 'x' } })
      expect(ids(store)).toEqual(['m1', 'm2'])
      expect(store.getState().lastSeq).toBe(5)
    })

    it('prependHistory puts older messages first, in order, without duplicates', () => {
      const store = withChat(range(5, 7))
      store.getState().prependHistory([...range(2, 4), msg(5), msg(4)])
      expect(ids(store)).toEqual(['m2', 'm3', 'm4', 'm5', 'm6', 'm7'])
    })

    it('prependHistory without a game changes nothing', () => {
      const store = createGameStore()
      store.getState().prependHistory(range(1, 2))
      expect(store.getState().game).toBeNull()
    })

    it('the history is not done after a snapshot of 50 messages', () => {
      const store = withChat(range(1, 50))
      expect(store.getState().game?.chatHistoryDone).toBe(false)
    })

    it('the history is done after a snapshot of 10 messages', () => {
      const store = withChat(range(1, 10))
      expect(store.getState().game?.chatHistoryDone).toBe(true)
    })

    it('a full page keeps the history open, a short one finishes it', () => {
      const store = withChat(range(101, 150))
      store.getState().prependHistory(range(51, 100))
      expect(store.getState().game?.chatHistoryDone).toBe(false)
      store.getState().prependHistory(range(41, 50))
      expect(store.getState().game?.chatHistoryDone).toBe(true)
      expect(store.getState().game?.chat).toHaveLength(110)
    })

    it('a new snapshot resets chatHistoryDone', () => {
      const store = withChat(range(1, 10))
      const snap = snapshot()
      if (snap.game === null) throw new Error('no game')
      snap.game = { ...snap.game, chat: range(1, 50) }
      store.getState().applySnapshot(snap)
      expect(store.getState().game?.chatHistoryDone).toBe(false)
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

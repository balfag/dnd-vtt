import { describe, expect, it } from 'vitest'
import type { PlayerInfo, Snapshot } from '../../shared/protocol'
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

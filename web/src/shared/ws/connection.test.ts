import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createGameStore } from '../../features/game/store'
import { GameConnection } from './connection'
import { CommandError } from './errors'
import { fakeSocketFactory, type FakeSocket } from './fakeSocket'

const snapshot = (seq: number, gameSessionId = 'g1') => ({
  t: 'sync.snapshot',
  d: {
    seq,
    me: { accountId: 'a1', isGm: false, playerId: 'p2' },
    game: {
      gameSessionId,
      startedAt: 1000,
      campaign: { id: 'c1', name: 'Campaign' },
      players: [
        { id: 'p1', displayName: 'GM', color: '#aa0000', role: 'gm', online: true },
        { id: 'p2', displayName: 'Ann', color: '#00aa00', role: 'player', online: true },
      ],
      activeSceneId: null,
      scenes: [],
      scene: null,
      chat: [],
    },
  },
})

const presence = (seq: number, online: boolean) => ({
  t: 'presence.update',
  seq,
  d: { playerId: 'p2', online },
})

function setup() {
  const store = createGameStore()
  const { factory, sockets } = fakeSocketFactory()
  const conn = new GameConnection({
    store,
    url: 'ws://x/ws',
    createSocket: factory,
    random: () => 0.5,
  })
  const last = (): FakeSocket => {
    const s = sockets[sockets.length - 1]
    if (s === undefined) throw new Error('no socket yet')
    return s
  }
  /** syncFresh runs the first connect up to synced with a snapshot at seq. */
  const syncFresh = (seq = 0) => {
    conn.start()
    last().open()
    last().receive({ t: 'ack', re: 'h-1', d: { resumed: false } })
    last().receive(snapshot(seq))
  }
  return { store, conn, sockets, last, syncFresh }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('GameConnection', () => {
  it('connects, says hello and takes the snapshot', () => {
    const { store, conn, last, sockets } = setup()
    conn.start()
    expect(store.getState().connection).toEqual({ state: 'connecting', attempt: 0 })
    expect(sockets).toHaveLength(1)
    last().open()
    expect(last().sent).toEqual([{ t: 'sync.hello', id: 'h-1', d: {} }])
    last().receive({ t: 'ack', re: 'h-1', d: { resumed: false } })
    expect(store.getState().connection.state).toBe('connecting')
    last().receive(snapshot(3))
    expect(store.getState().connection).toEqual({ state: 'synced' })
    expect(store.getState().game?.players).toHaveLength(2)
    expect(store.getState().lastSeq).toBe(3)
  })

  it('resumes after a drop without a new snapshot', () => {
    const { store, last, sockets, syncFresh } = setup()
    syncFresh(3)
    const game = store.getState().game
    last().serverClose(1006)
    expect(store.getState().connection).toEqual({ state: 'reconnecting', attempt: 1, delayMs: 500 })
    vi.advanceTimersByTime(499)
    expect(sockets).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(sockets).toHaveLength(2)
    expect(store.getState().connection).toEqual({ state: 'connecting', attempt: 1 })
    last().open()
    expect(last().sent).toEqual([
      { t: 'sync.hello', id: 'h-2', d: { gameSessionId: 'g1', lastSeq: 3 } },
    ])
    last().receive({ t: 'ack', re: 'h-2', d: { resumed: true } })
    expect(store.getState().connection).toEqual({ state: 'synced' })
    expect(store.getState().game).toBe(game)
  })

  it('applies events in order, ignores repeats and resyncs on a gap', () => {
    const { store, last, sockets, syncFresh } = setup()
    syncFresh(0)
    last().receive(presence(1, false))
    last().receive(presence(2, true))
    expect(store.getState().lastSeq).toBe(2)
    last().receive(presence(2, false))
    expect(store.getState().game?.players[1]?.online).toBe(true)
    const first = last()
    first.receive(presence(4, false))
    expect(first.closedWith).toBe(1000)
    expect(sockets).toHaveLength(2)
    expect(store.getState().connection).toEqual({ state: 'connecting', attempt: 1 })
    // The late close of the old socket changes nothing.
    first.serverClose(1000)
    expect(sockets).toHaveLength(2)
    vi.advanceTimersByTime(60000)
    expect(sockets).toHaveLength(2)
  })

  it('grows the pause on repeated failures and resets it after synced', () => {
    const { store, conn, last, sockets } = setup()
    conn.start()
    const delays: number[] = []
    for (let i = 0; i < 3; i++) {
      last().serverClose(1006)
      const c = store.getState().connection
      if (c.state !== 'reconnecting') throw new Error('expected reconnecting')
      delays.push(c.delayMs)
      vi.advanceTimersByTime(c.delayMs)
    }
    expect(delays).toEqual([500, 1000, 2000])
    expect(sockets).toHaveLength(4)
    last().open()
    last().receive({ t: 'ack', re: 'h-1', d: { resumed: false } })
    last().receive(snapshot(0))
    expect(store.getState().connection.state).toBe('synced')
    last().serverClose(1006)
    expect(store.getState().connection).toEqual({ state: 'reconnecting', attempt: 1, delayMs: 500 })
  })

  it.each([
    [4001, 'unauthorized'],
    [4003, 'removed'],
    [4004, 'gameEnded'],
  ] as const)('does not reconnect after %i', (code, reason) => {
    const { store, last, sockets, syncFresh } = setup()
    syncFresh(0)
    last().serverClose(code)
    expect(store.getState().connection).toEqual({ state: 'closed', reason })
    vi.advanceTimersByTime(60000)
    expect(sockets).toHaveLength(1)
  })

  it('logs an error for client-fault close codes and reconnects', () => {
    const { store, last, syncFresh } = setup()
    const err = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    syncFresh(0)
    last().serverClose(1009)
    expect(err).toHaveBeenCalledOnce()
    expect(store.getState().connection.state).toBe('reconnecting')
  })

  it('pings every 20 seconds while synced and stops after close', () => {
    const { last, sockets, syncFresh } = setup()
    syncFresh(0)
    const first = last()
    vi.advanceTimersByTime(19999)
    expect(first.sent).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(first.sent[1]).toMatchObject({ t: 'ping', id: 'p-1' })
    vi.advanceTimersByTime(20000)
    expect(first.sent[2]).toMatchObject({ t: 'ping', id: 'p-2' })
    first.serverClose(1006)
    vi.advanceTimersByTime(100000)
    expect(first.sent).toHaveLength(3)
    expect(sockets).toHaveLength(2)
  })

  it('resolves, rejects and times out commands', async () => {
    const { last, conn, syncFresh } = setup()
    syncFresh(0)

    const ok = conn.command('x.y', { a: 1 })
    expect(last().sent[1]).toEqual({ t: 'x.y', id: 'c-1', d: { a: 1 } })
    last().receive({ t: 'ack', re: 'c-1', d: { ok: true } })
    await expect(ok).resolves.toEqual({ ok: true })

    const bad = conn.command('x.z')
    const badResult = expect(bad).rejects.toSatisfy(
      (e: unknown) => e instanceof CommandError && e.code === 'forbidden',
    )
    last().receive({ t: 'error', re: 'c-2', d: { code: 'forbidden', message: 'no' } })
    await badResult

    const slow = conn.command('x.slow')
    const slowResult = expect(slow).rejects.toMatchObject({ code: 'timeout' })
    vi.advanceTimersByTime(10000)
    await slowResult
  })

  it('rejects pending commands on disconnect and refuses new ones while reconnecting', async () => {
    const { last, conn, syncFresh } = setup()
    syncFresh(0)
    const p = conn.command('x.y')
    const result = expect(p).rejects.toMatchObject({ code: 'disconnected' })
    last().serverClose(1006)
    await result
    await expect(conn.command('x.y')).rejects.toMatchObject({ code: 'disconnected' })
  })

  it('sends transient messages only when synced', () => {
    const { conn, last, sockets } = setup()
    conn.start()
    conn.transient('cursor', { x: 1 })
    last().open()
    conn.transient('cursor', { x: 1 })
    expect(last().sent).toHaveLength(1) // only the hello
    last().receive({ t: 'ack', re: 'h-1', d: { resumed: false } })
    last().receive(snapshot(0))
    conn.transient('cursor', { x: 2 })
    expect(last().sent[1]).toEqual({ t: 'cursor', d: { x: 2 } })
    expect(last().sent[1]).not.toHaveProperty('id')
    expect(sockets).toHaveLength(1)
  })

  it('warns about garbage and carries on', () => {
    const { store, last, syncFresh } = setup()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    syncFresh(0)
    const before = store.getState()
    last().receiveRaw('not json')
    last().receive({ x: 1 })
    expect(warn).toHaveBeenCalledTimes(2)
    expect(store.getState()).toBe(before)
    expect(last().closedWith).toBeUndefined()
  })

  it('stops cleanly and can start again', () => {
    const { store, conn, last, sockets, syncFresh } = setup()
    syncFresh(0)
    const first = last()
    conn.stop()
    expect(first.closedWith).toBe(1000)
    expect(store.getState().connection).toEqual({ state: 'idle' })
    first.serverClose(1000)
    vi.advanceTimersByTime(60000)
    expect(sockets).toHaveLength(1)
    expect(store.getState().connection).toEqual({ state: 'idle' })
    conn.start()
    expect(sockets).toHaveLength(2)
  })

  it('takes a new snapshot at any time, for example after the game changed', () => {
    const { store, last, syncFresh } = setup()
    syncFresh(7)
    last().receive(snapshot(0, 'g2'))
    expect(store.getState().game?.gameSessionId).toBe('g2')
    expect(store.getState().lastSeq).toBe(0)
    last().receive(presence(1, false))
    expect(store.getState().lastSeq).toBe(1)
    expect(store.getState().game?.players[1]?.online).toBe(false)
  })
})

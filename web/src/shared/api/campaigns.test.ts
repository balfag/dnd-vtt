import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
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
} from './campaigns'

const campaign = { id: 'c1', name: 'Strahd', createdAt: 1000, activeGameSession: null }
const player = {
  id: 'p1',
  accountId: 'a1',
  accountLogin: 'anna',
  displayName: 'Anna',
  color: '#e53935',
  role: 'player',
  removed: false,
  online: true,
}

function stubFetch(res: () => Response) {
  const fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(res()))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function lastCall(fetchMock: ReturnType<typeof stubFetch>) {
  const call = fetchMock.mock.calls[fetchMock.mock.calls.length - 1]
  if (call === undefined) throw new Error('fetch was not called')
  const input = call[0]
  if (typeof input !== 'string') throw new Error('fetch was called with a non-string input')
  return { path: input, init: call[1] }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('campaign api calls', () => {
  it('listCampaigns', async () => {
    const f = stubFetch(() => Response.json([campaign]))
    expect(await listCampaigns()).toEqual([campaign])
    expect(lastCall(f).path).toBe('/api/campaigns')
    expect(lastCall(f).init?.method).toBe('GET')
  })

  it('createCampaign posts the name', async () => {
    const f = stubFetch(() => Response.json(campaign, { status: 201 }))
    await createCampaign('Strahd')
    expect(lastCall(f).path).toBe('/api/campaigns')
    expect(lastCall(f).init).toMatchObject({ method: 'POST', body: '{"name":"Strahd"}' })
  })

  it('renameCampaign patches with the name', async () => {
    const f = stubFetch(() => Response.json(campaign))
    await renameCampaign('c1', 'New')
    expect(lastCall(f).path).toBe('/api/campaigns/c1')
    expect(lastCall(f).init).toMatchObject({ method: 'PATCH', body: '{"name":"New"}' })
  })

  it('startGame', async () => {
    const f = stubFetch(() =>
      Response.json({ id: 'g1', campaignId: 'c1', startedAt: 5, link: 'http://x/play/t' }),
    )
    const game = await startGame('c1')
    expect(game.link).toBe('http://x/play/t')
    expect(lastCall(f).path).toBe('/api/campaigns/c1/game-sessions')
    expect(lastCall(f).init?.method).toBe('POST')
  })

  it('endGame and removePlayer accept 204', async () => {
    const f = stubFetch(() => new Response(null, { status: 204 }))
    await expect(endGame('g1')).resolves.toBeUndefined()
    expect(lastCall(f).path).toBe('/api/game-sessions/g1/end')
    await expect(removePlayer('p1')).resolves.toBeUndefined()
    expect(lastCall(f).path).toBe('/api/players/p1/remove')
    expect(lastCall(f).init?.method).toBe('POST')
  })

  it('rotateLink', async () => {
    const f = stubFetch(() => Response.json({ link: 'http://x/play/new' }))
    expect(await rotateLink('g1')).toEqual({ link: 'http://x/play/new' })
    expect(lastCall(f).path).toBe('/api/game-sessions/g1/link')
  })

  it('listPlayers', async () => {
    const f = stubFetch(() => Response.json([player]))
    expect(await listPlayers('c1')).toEqual([player])
    expect(lastCall(f).path).toBe('/api/campaigns/c1/players')
  })

  it('createPasswordReset', async () => {
    const f = stubFetch(() =>
      Response.json({ link: 'http://x/reset/t', expiresAt: 9 }, { status: 201 }),
    )
    expect(await createPasswordReset('a1')).toEqual({ link: 'http://x/reset/t', expiresAt: 9 })
    expect(lastCall(f).path).toBe('/api/accounts/a1/password-resets')
  })

  it('escapes ids in the path', async () => {
    const f = stubFetch(() => new Response(null, { status: 204 }))
    await endGame('a/b?c')
    expect(lastCall(f).path).toBe('/api/game-sessions/a%2Fb%3Fc/end')
    await removePlayer('../x')
    expect(lastCall(f).path).toBe('/api/players/..%2Fx/remove')
  })
})

async function failure(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise
  } catch (err) {
    if (err instanceof ApiError) return err
    throw err
  }
  throw new Error('expected the call to fail')
}

describe('campaign guards', () => {
  it('rejects a player with a bad color', async () => {
    stubFetch(() => Response.json([{ ...player, color: 'red' }]))
    expect((await failure(listPlayers('c1'))).code).toBe('unknown')
  })

  it('rejects a player with a color in upper case', async () => {
    stubFetch(() => Response.json([{ ...player, color: '#E53935' }]))
    expect((await failure(listPlayers('c1'))).code).toBe('unknown')
  })

  it('needs activeGameSession to be present: null or an object', async () => {
    stubFetch(() => Response.json([{ id: 'c1', name: 'x', createdAt: 1 }]))
    expect((await failure(listCampaigns())).code).toBe('unknown')

    stubFetch(() => Response.json([{ ...campaign, activeGameSession: { id: 'g1', startedAt: 7 } }]))
    expect((await listCampaigns())[0]?.activeGameSession).toEqual({ id: 'g1', startedAt: 7 })

    stubFetch(() => Response.json([{ ...campaign, activeGameSession: 'g1' }]))
    expect((await failure(listCampaigns())).code).toBe('unknown')
  })

  it('rejects a player of an unknown role', async () => {
    stubFetch(() => Response.json([{ ...player, role: 'spectator' }]))
    expect((await failure(listPlayers('c1'))).code).toBe('unknown')
  })

  it('rejects a started game without a link', async () => {
    stubFetch(() => Response.json({ id: 'g1', campaignId: 'c1', startedAt: 5 }))
    expect((await failure(startGame('c1'))).code).toBe('unknown')
  })
})

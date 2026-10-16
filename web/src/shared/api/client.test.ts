import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiRequest } from './client'

function stubFetch(impl: () => Response | Promise<Response>) {
  const fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(impl()))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function isNamed(v: unknown): v is { name: string } {
  return typeof v === 'object' && v !== null && 'name' in v && typeof v.name === 'string'
}

async function failure(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise
  } catch (err) {
    if (err instanceof ApiError) return err
    throw err
  }
  throw new Error('expected the request to fail')
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('apiRequest', () => {
  it('returns undefined for 204', async () => {
    stubFetch(() => new Response(null, { status: 204 }))
    expect(await apiRequest('POST', '/x')).toBeUndefined()
  })

  it('returns the body when the guard accepts it', async () => {
    stubFetch(() => Response.json({ name: 'anna' }))
    expect(await apiRequest('GET', '/x', { guard: isNamed })).toEqual({ name: 'anna' })
  })

  it('throws unknown when the guard rejects the body', async () => {
    stubFetch(() => Response.json({ nope: 1 }))
    const err = await failure(apiRequest('GET', '/x', { guard: isNamed }))
    expect([err.status, err.code]).toEqual([200, 'unknown'])
  })

  it('takes the code from the error body', async () => {
    stubFetch(() =>
      Response.json({ error: { code: 'unauthorized', message: 'm' } }, { status: 401 }),
    )
    const err = await failure(apiRequest('GET', '/x'))
    expect([err.status, err.code]).toEqual([401, 'unauthorized'])
  })

  it('throws unknown for a failure that is not JSON', async () => {
    stubFetch(() => new Response('oops', { status: 500 }))
    const err = await failure(apiRequest('GET', '/x'))
    expect([err.status, err.code]).toEqual([500, 'unknown'])
  })

  it('throws network when fetch fails', async () => {
    stubFetch(() => Promise.reject(new TypeError('failed to fetch')))
    const err = await failure(apiRequest('GET', '/x'))
    expect([err.status, err.code]).toEqual([0, 'network'])
  })

  it('sends PATCH with a JSON body', async () => {
    const fetchMock = stubFetch(() => Response.json({ ok: true }))
    await apiRequest('PATCH', '/x', { body: { name: 'n' } })
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: '{"name":"n"}',
    })
  })

  it('sends JSON with a Content-Type only when there is a body', async () => {
    const fetchMock = stubFetch(() => new Response(null, { status: 204 }))

    await apiRequest('POST', '/x', { body: { a: 1 } })
    await apiRequest('POST', '/y')

    const [withBody, withoutBody] = fetchMock.mock.calls.map((call) => call[1])
    expect(withBody).toMatchObject({
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: '{"a":1}',
    })
    expect(withoutBody?.headers).toBeUndefined()
    expect(withoutBody?.body).toBeUndefined()
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchHealth } from './health'

function stubFetch(body: unknown, status = 200) {
  const fn = vi.fn(() => Promise.resolve(Response.json(body, { status })))
  vi.stubGlobal('fetch', fn)
  return fn
}

describe('fetchHealth', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('resolves with the parsed body', async () => {
    const fetchMock = stubFetch({ status: 'ok', version: '1.2.3' })
    await expect(fetchHealth()).resolves.toEqual({ status: 'ok', version: '1.2.3' })
    expect(fetchMock).toHaveBeenCalledWith('/api/health', { signal: undefined })
  })

  it('rejects on a non-2xx status', async () => {
    stubFetch({}, 500)
    await expect(fetchHealth()).rejects.toThrow('unexpected status 500')
  })

  it('rejects on a malformed body', async () => {
    stubFetch({ status: 'down' })
    await expect(fetchHealth()).rejects.toThrow('malformed response')
  })
})

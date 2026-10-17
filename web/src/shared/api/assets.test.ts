import { afterEach, describe, expect, it, vi } from 'vitest'
import { isAsset, listAssets, uploadAsset, type Asset } from './assets'
import { ApiError } from './client'

const asset: Asset = {
  id: 'a1',
  campaignId: 'c1',
  kind: 'map',
  mime: 'image/webp',
  size: 1834221,
  width: 3000,
  height: 2000,
  originalName: 'tavern.webp',
  createdAt: 1760530000000,
  url: '/assets/a1',
}

class FakeXhr {
  static last: FakeXhr | null = null
  upload: { onprogress: ((e: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  onabort: (() => void) | null = null
  status = 0
  responseText = ''
  method = ''
  url = ''
  body: FormData | null = null

  constructor() {
    FakeXhr.last = this
  }
  open(method: string, url: string) {
    this.method = method
    this.url = url
  }
  send(body: FormData) {
    this.body = body
  }
  progress(loaded: number, total: number) {
    this.upload.onprogress?.({ lengthComputable: true, loaded, total } as ProgressEvent)
  }
  reply(status: number, body: unknown) {
    this.status = status
    this.responseText = typeof body === 'string' ? body : JSON.stringify(body)
    this.onload?.()
  }
}

function lastXhr(): FakeXhr {
  if (FakeXhr.last === null) throw new Error('no request was made')
  return FakeXhr.last
}

function file(): File {
  return new File(['x'], 'tavern.webp', { type: 'image/webp' })
}

afterEach(() => {
  vi.unstubAllGlobals()
  FakeXhr.last = null
})

describe('isAsset', () => {
  it('accepts an asset and rejects a broken one', () => {
    expect(isAsset(asset)).toBe(true)
    expect(isAsset({ ...asset, size: '1' })).toBe(false)
    expect(isAsset({ ...asset, url: undefined })).toBe(false)
    expect(isAsset(null)).toBe(false)
    expect(isAsset([asset])).toBe(false)
  })
})

describe('listAssets', () => {
  function stubFetch(body: unknown) {
    const mock = vi.fn<typeof fetch>(() =>
      Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    )
    vi.stubGlobal('fetch', mock)
    return mock
  }

  it('gets the assets of the campaign', async () => {
    const mock = stubFetch([asset])
    await expect(listAssets('c 1')).resolves.toEqual([asset])
    expect(mock.mock.calls[0]?.[0]).toBe('/api/campaigns/c%201/assets')
  })

  it('rejects a list with a broken asset', async () => {
    stubFetch([asset, { id: 'a2' }])
    await expect(listAssets('c1')).rejects.toMatchObject({ code: 'unknown' })
  })
})

describe('uploadAsset', () => {
  it('posts kind before file', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const done = uploadAsset('c1', file(), 'map')
    const xhr = lastXhr()
    expect(xhr.method).toBe('POST')
    expect(xhr.url).toBe('/api/campaigns/c1/assets')
    expect([...(xhr.body?.keys() ?? [])]).toEqual(['kind', 'file'])
    expect(xhr.body?.get('kind')).toBe('map')
    xhr.reply(201, asset)
    await expect(done).resolves.toEqual(asset)
  })

  it('reports the progress from 0 to 1', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const seen: number[] = []
    const done = uploadAsset('c1', file(), 'map', (p) => seen.push(p))
    lastXhr().progress(25, 100)
    lastXhr().progress(100, 100)
    lastXhr().reply(200, asset)
    await done
    expect(seen).toEqual([0.25, 1])
  })

  it('maps 413 and 415 to ApiError with their codes', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const big = uploadAsset('c1', file(), 'map')
    lastXhr().reply(413, { error: { code: 'too_large', message: 'x' } })
    await expect(big).rejects.toEqual(new ApiError(413, 'too_large'))

    const bad = uploadAsset('c1', file(), 'map')
    lastXhr().reply(415, '')
    await expect(bad).rejects.toEqual(new ApiError(415, 'unsupported_media_type'))
  })

  it('takes the code of another failure from the body', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const done = uploadAsset('c1', file(), 'map')
    lastXhr().reply(400, { error: { code: 'bad_request', message: 'x' } })
    await expect(done).rejects.toEqual(new ApiError(400, 'bad_request'))
  })

  it('rejects a reply that is not an asset', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const done = uploadAsset('c1', file(), 'map')
    lastXhr().reply(201, { id: 'a1' })
    await expect(done).rejects.toEqual(new ApiError(201, 'unknown'))
  })

  it('reports a network failure', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const done = uploadAsset('c1', file(), 'map')
    lastXhr().onerror?.()
    await expect(done).rejects.toEqual(new ApiError(0, 'network'))
  })
})

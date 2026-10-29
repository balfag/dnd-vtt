import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import {
  createWikiPage,
  deleteWikiPage,
  getBacklinks,
  getWikiPage,
  getWikiTree,
  isPageNode,
  isPageRef,
  isSaved,
  isWikiPage,
  moveWikiPage,
  saveWikiPage,
  searchMentions,
  type PageNode,
  type WikiPage,
} from './wiki'

const node: PageNode = {
  id: 'p1',
  parentId: null,
  kind: 'page',
  title: 'Таверна',
  icon: '🍺',
  position: 0,
  recordCount: 0,
}

const page: WikiPage = {
  id: 'p1',
  campaignId: 'c1',
  parentId: null,
  kind: 'page',
  isRecord: false,
  title: 'Таверна',
  icon: '🍺',
  coverAssetId: null,
  content: { type: 'doc', content: [{ type: 'paragraph' }] },
  props: {},
  tags: [{ id: 't1', name: 'NPC' }],
  path: [{ id: 'p0', title: 'Локации', icon: '', kind: 'page' }],
  refs: { 'scene:s1': { label: 'Склеп', exists: true } },
  version: 3,
  createdAt: 1760530000000,
  updatedAt: 1760530000001,
}

function stubFetch(status: number, body?: unknown) {
  const mock = vi.fn<typeof fetch>(() =>
    Promise.resolve(new Response(body === undefined ? null : JSON.stringify(body), { status })),
  )
  vi.stubGlobal('fetch', mock)
  return mock
}

function call(mock: ReturnType<typeof stubFetch>): { url: string; init: RequestInit } {
  const [url, init] = mock.mock.calls[0] ?? []
  return { url: String(url), init: init ?? {} }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('shape checks', () => {
  it('accept correct objects', () => {
    expect(isPageNode(node)).toBe(true)
    expect(isPageNode({ ...node, parentId: 'p0', kind: 'database', recordCount: 4 })).toBe(true)
    expect(isPageRef(page.path[0])).toBe(true)
    expect(isWikiPage(page)).toBe(true)
    expect(isSaved({ version: 4, updatedAt: 1, refs: {} })).toBe(true)
  })

  it('reject a missing field or a wrong type', () => {
    expect(isPageNode({ ...node, recordCount: undefined })).toBe(false)
    expect(isPageNode({ ...node, kind: 'folder' })).toBe(false)
    expect(isPageNode({ ...node, position: '0' })).toBe(false)
    expect(isPageNode(null)).toBe(false)
    expect(isPageRef({ id: 'p0', title: 'x', icon: '' })).toBe(false)
    expect(isWikiPage({ ...page, version: '3' })).toBe(false)
    expect(isWikiPage({ ...page, coverAssetId: 5 })).toBe(false)
    expect(isWikiPage({ ...page, path: [{ id: 'p0' }] })).toBe(false)
    expect(isWikiPage({ ...page, refs: { 'scene:s1': { label: 'x' } } })).toBe(false)
    const { content: _content, ...noContent } = page
    expect(isWikiPage(noContent)).toBe(false)
    expect(isSaved({ version: 4, refs: {} })).toBe(false)
  })
})

describe('getWikiTree', () => {
  it('reads the pages of the campaign', async () => {
    const mock = stubFetch(200, { pages: [node] })
    await expect(getWikiTree('c 1')).resolves.toEqual([node])
    expect(call(mock).url).toBe('/api/campaigns/c%201/wiki/tree')
  })

  it('rejects a tree with a broken node', async () => {
    stubFetch(200, { pages: [node, { id: 'x' }] })
    await expect(getWikiTree('c1')).rejects.toMatchObject({ code: 'unknown' })
  })
})

describe('pages', () => {
  it('creates a page', async () => {
    const mock = stubFetch(201, page)
    await expect(createWikiPage('c1', { parentId: 'p0', title: 'T' })).resolves.toEqual(page)
    const { url, init } = call(mock)
    expect(url).toBe('/api/campaigns/c1/wiki/pages')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ parentId: 'p0', title: 'T' }))
  })

  it('gets a page', async () => {
    const mock = stubFetch(200, page)
    await expect(getWikiPage('p1')).resolves.toEqual(page)
    expect(call(mock).url).toBe('/api/wiki/pages/p1')
  })

  it('saves a page with PUT and turns 409 into a conflict', async () => {
    const saved = { version: 4, updatedAt: 2, refs: {} }
    const mock = stubFetch(200, saved)
    const body = { version: 3, title: 'T', icon: '', coverAssetId: null, content: {} }
    await expect(saveWikiPage('p1', body)).resolves.toEqual(saved)
    expect(call(mock).init.method).toBe('PUT')

    stubFetch(409, { error: { code: 'conflict', message: 'x' } })
    await expect(saveWikiPage('p1', body)).rejects.toEqual(new ApiError(409, 'conflict'))
  })

  it('passes keepalive on', async () => {
    const mock = stubFetch(200, { version: 4, updatedAt: 2, refs: {} })
    await saveWikiPage(
      'p1',
      { version: 3, title: '', icon: '', coverAssetId: null, content: {} },
      { keepalive: true },
    )
    expect(call(mock).init.keepalive).toBe(true)
  })

  it('moves and deletes', async () => {
    const mock = stubFetch(204)
    await moveWikiPage('p1', null, 2)
    expect(call(mock).url).toBe('/api/wiki/pages/p1/move')
    expect(call(mock).init.body).toBe(JSON.stringify({ parentId: null, index: 2 }))

    const del = stubFetch(204)
    await deleteWikiPage('p1')
    expect(call(del).init.method).toBe('DELETE')
  })
})

describe('mentions and backlinks', () => {
  it('searches mentions with an escaped query', async () => {
    const hit = { kind: 'scene', id: 's1', label: 'Склеп', detail: '' }
    const mock = stubFetch(200, [hit])
    await expect(searchMentions('c1', 'а б&')).resolves.toEqual([hit])
    expect(call(mock).url).toBe(
      `/api/campaigns/c1/wiki/mention-search?q=${encodeURIComponent('а б&')}`,
    )
  })

  it('rejects a hit of an unknown kind', async () => {
    stubFetch(200, [{ kind: 'npc', id: 's1', label: 'x', detail: '' }])
    await expect(searchMentions('c1', '')).rejects.toMatchObject({ code: 'unknown' })
  })

  it('reads backlinks', async () => {
    const ref = { id: 'p2', title: 'Сессия 9', icon: '📓', kind: 'page' }
    const mock = stubFetch(200, [ref])
    await expect(getBacklinks('p1')).resolves.toEqual([ref])
    expect(call(mock).url).toBe('/api/wiki/pages/p1/backlinks')
  })
})

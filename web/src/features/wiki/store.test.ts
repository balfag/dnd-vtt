import { describe, expect, it } from 'vitest'
import type { PageNode } from '../../shared/api/wiki'
import { createWikiStore } from './store'

function node(id: string, parentId: string | null): PageNode {
  return { id, parentId, kind: 'page', title: id, icon: '', position: 0, recordCount: 0 }
}

// a ─ b ─ c, d
const tree = [node('a', null), node('b', 'a'), node('c', 'b'), node('d', null)]

describe('wiki store', () => {
  it('adds an opened page to the open ones once and makes it active', () => {
    const store = createWikiStore()
    store.getState().setTree(tree)
    store.getState().openPage('b')
    store.getState().openPage('d')
    store.getState().openPage('b')
    expect(store.getState().open).toEqual(['b', 'd'])
    expect(store.getState().active).toBe('b')
  })

  it('closing the active page makes the last open one active', () => {
    const store = createWikiStore()
    store.getState().openPage('b')
    store.getState().openPage('d')
    store.getState().closePage('d')
    expect(store.getState().open).toEqual(['b'])
    expect(store.getState().active).toBe('b')
    store.getState().closePage('b')
    expect(store.getState().active).toBeNull()
  })

  it('removing a page closes its panels and those of its descendants', () => {
    const store = createWikiStore()
    store.getState().setTree(tree)
    for (const id of ['a', 'c', 'd']) store.getState().openPage(id)
    store.getState().removePage('a')
    expect(store.getState().open).toEqual(['d'])
    expect(store.getState().active).toBe('d')
    expect(store.getState().tree?.map((n) => n.id)).toEqual(['d'])
  })

  it('retitles a page in the tree at once', () => {
    const store = createWikiStore()
    store.getState().setTree(tree)
    store.getState().retitle('b', 'New', '🍺')
    expect(store.getState().tree?.find((n) => n.id === 'b')).toMatchObject({
      title: 'New',
      icon: '🍺',
    })
    store.getState().retitle('b', 'Newer')
    expect(store.getState().tree?.find((n) => n.id === 'b')?.icon).toBe('🍺')
  })

  it('reset forgets everything', () => {
    const store = createWikiStore()
    store.getState().setTree(tree)
    store.getState().openPage('a')
    store.getState().reset()
    expect(store.getState()).toMatchObject({ tree: null, open: [], active: null })
  })
})

import { describe, expect, it } from 'vitest'
import type { PageNode } from '../../../shared/api/wiki'
import {
  buildTree,
  canMoveInto,
  childrenOf,
  descendantIds,
  dropFor,
  parentCandidates,
  visibleRows,
} from './tree'

function node(id: string, parentId: string | null, position: number, extra?: Partial<PageNode>) {
  const n: PageNode = {
    id,
    parentId,
    kind: 'page',
    title: id.toUpperCase(),
    icon: '',
    position,
    recordCount: 0,
    ...extra,
  }
  return n
}

//  a            d (database)
//  ├ b          e
//  │ └ c
//  └ f
const nodes: PageNode[] = [
  node('f', 'a', 1),
  node('c', 'b', 0),
  node('d', null, 1, { kind: 'database', recordCount: 3 }),
  node('b', 'a', 0),
  node('a', null, 0),
  node('e', null, 2),
]

describe('buildTree', () => {
  it('orders siblings by position', () => {
    const tree = buildTree(nodes)
    expect(tree.map((i) => i.node.id)).toEqual(['a', 'd', 'e'])
    expect(tree[0]?.children.map((i) => i.node.id)).toEqual(['b', 'f'])
    expect(tree[0]?.children[0]?.children.map((i) => i.node.id)).toEqual(['c'])
  })

  it('keeps a database with its record count', () => {
    expect(buildTree(nodes)[1]?.node.recordCount).toBe(3)
  })

  it('shows a page of an unknown parent at the root', () => {
    const tree = buildTree([node('x', 'gone', 0), node('y', null, 1)])
    expect(tree.map((i) => i.node.id)).toEqual(['x', 'y'])
  })

  it('survives a cycle', () => {
    expect(() => buildTree([node('x', 'y', 0), node('y', 'x', 0)])).not.toThrow()
  })
})

describe('descendantIds and visibleRows', () => {
  it('lists the pages below at any depth', () => {
    expect(descendantIds(nodes, 'a').sort()).toEqual(['b', 'c', 'f'])
    expect(descendantIds(nodes, 'e')).toEqual([])
  })

  it('shows the children of an open page only', () => {
    const closed = visibleRows(buildTree(nodes), new Set())
    expect(closed.map((r) => r.node.id)).toEqual(['a', 'd', 'e'])
    expect(closed[0]?.hasChildren).toBe(true)
    const open = visibleRows(buildTree(nodes), new Set(['a', 'b']))
    expect(open.map((r) => [r.node.id, r.depth])).toEqual([
      ['a', 0],
      ['b', 1],
      ['c', 2],
      ['f', 1],
      ['d', 0],
      ['e', 0],
    ])
  })
})

describe('dropFor', () => {
  it('on a row makes the page its last child', () => {
    expect(dropFor(nodes, 'e', 'a', 'on')).toEqual({ parentId: 'a', index: 2 })
    expect(dropFor(nodes, 'e', 'c', 'on')).toEqual({ parentId: 'c', index: 0 })
  })

  it('above a row puts the page at the place of the row', () => {
    expect(dropFor(nodes, 'e', 'b', 'before')).toEqual({ parentId: 'a', index: 0 })
    expect(dropFor(nodes, 'c', 'a', 'before')).toEqual({ parentId: null, index: 0 })
  })

  it('below a row puts the page one place after it', () => {
    expect(dropFor(nodes, 'e', 'b', 'after')).toEqual({ parentId: 'a', index: 1 })
    expect(dropFor(nodes, 'c', 'e', 'after')).toEqual({ parentId: null, index: 3 })
  })

  it('counts the siblings without the dragged page that stood higher', () => {
    // a, d, e at the root: a goes below e
    expect(dropFor(nodes, 'a', 'e', 'after')).toEqual({ parentId: null, index: 2 })
    // b goes below f in a
    expect(dropFor(nodes, 'b', 'f', 'after')).toEqual({ parentId: 'a', index: 1 })
    // a goes above e: after d
    expect(dropFor(nodes, 'a', 'e', 'before')).toEqual({ parentId: null, index: 1 })
    // a page that stood lower does not shift the row
    expect(dropFor(nodes, 'e', 'a', 'before')).toEqual({ parentId: null, index: 0 })
  })

  it('on a parent that already holds the page puts it last without counting itself', () => {
    expect(dropFor(nodes, 'b', 'a', 'on')).toEqual({ parentId: 'a', index: 1 })
  })

  it('refuses a drop into the page itself and into its descendants', () => {
    expect(dropFor(nodes, 'a', 'a', 'on')).toBeNull()
    expect(dropFor(nodes, 'a', 'b', 'on')).toBeNull()
    expect(dropFor(nodes, 'a', 'c', 'on')).toBeNull()
    expect(dropFor(nodes, 'a', 'c', 'before')).toBeNull()
    expect(dropFor(nodes, 'a', 'f', 'after')).toBeNull()
  })

  it('refuses to make a page a child of a database', () => {
    expect(dropFor(nodes, 'e', 'd', 'on')).toBeNull()
    expect(canMoveInto(nodes, 'e', 'd')).toBe(false)
    // next to the database is fine
    expect(dropFor(nodes, 'e', 'd', 'before')).toEqual({ parentId: null, index: 1 })
  })

  it('returns null when the page would stay where it is', () => {
    expect(dropFor(nodes, 'b', 'f', 'before')).toBeNull()
    expect(dropFor(nodes, 'f', 'b', 'after')).toBeNull()
    expect(dropFor(nodes, 'unknown', 'a', 'on')).toBeNull()
  })
})

describe('parentCandidates', () => {
  it('skips the page, its descendants and databases, and matches by title', () => {
    expect(parentCandidates(nodes, 'a', '').map((n) => n.id)).toEqual(['e'])
    expect(parentCandidates(nodes, 'e', '').map((n) => n.id)).toEqual(['a', 'b', 'c', 'f'])
    expect(parentCandidates(nodes, 'e', ' B ').map((n) => n.id)).toEqual(['b'])
  })

  it('childrenOf lists the roots in order', () => {
    expect(childrenOf(nodes, null).map((n) => n.id)).toEqual(['a', 'd', 'e'])
  })
})

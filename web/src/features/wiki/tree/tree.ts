import type { PageNode } from '../../../shared/api/wiki'

/** TreeItem is a page with its children in the order of the tree. */
export interface TreeItem {
  node: PageNode
  children: TreeItem[]
}

/** Row is a visible line of the tree. */
export interface Row {
  node: PageNode
  depth: number
  hasChildren: boolean
}

export type DropZone = 'before' | 'on' | 'after'

/** Drop is where a page lands: under parentId at index among its new siblings (the page itself not counted). */
export interface Drop {
  parentId: string | null
  index: number
}

function bySiblings(a: PageNode, b: PageNode): number {
  return a.position - b.position || a.id.localeCompare(b.id)
}

/** childrenOf lists the pages under parentId (null: the root) in order. A page whose parent is unknown counts as a root page. */
export function childrenOf(nodes: PageNode[], parentId: string | null): PageNode[] {
  const known = new Set(nodes.map((n) => n.id))
  return nodes
    .filter((n) =>
      parentId === null ? n.parentId === null || !known.has(n.parentId) : n.parentId === parentId,
    )
    .sort(bySiblings)
}

/** buildTree makes the nested tree of the flat list the server sends. */
export function buildTree(nodes: PageNode[]): TreeItem[] {
  const byParent = new Map<string | null, PageNode[]>()
  const known = new Set(nodes.map((n) => n.id))
  for (const n of nodes) {
    const key = n.parentId !== null && known.has(n.parentId) ? n.parentId : null
    const list = byParent.get(key)
    if (list === undefined) byParent.set(key, [n])
    else list.push(n)
  }
  const build = (parent: string | null, seen: Set<string>): TreeItem[] =>
    (byParent.get(parent) ?? [])
      .sort(bySiblings)
      .filter((n) => !seen.has(n.id))
      .map((node) => ({ node, children: build(node.id, new Set(seen).add(node.id)) }))
  return build(null, new Set())
}

/** descendantIds lists every page below id, at any depth. */
export function descendantIds(nodes: PageNode[], id: string): string[] {
  const result: string[] = []
  const queue = [id]
  while (queue.length > 0) {
    const head = queue.shift()
    for (const n of nodes) {
      if (n.parentId === head && !result.includes(n.id)) {
        result.push(n.id)
        queue.push(n.id)
      }
    }
  }
  return result
}

/** visibleRows flattens the tree to the lines shown when only the pages in expanded are open. */
export function visibleRows(items: TreeItem[], expanded: ReadonlySet<string>): Row[] {
  const rows: Row[] = []
  const walk = (list: TreeItem[], depth: number) => {
    for (const item of list) {
      rows.push({ node: item.node, depth, hasChildren: item.children.length > 0 })
      if (expanded.has(item.node.id)) walk(item.children, depth + 1)
    }
  }
  walk(items, 0)
  return rows
}

/** canMoveInto tells whether the page may become a child of parentId (null: the root). */
export function canMoveInto(nodes: PageNode[], id: string, parentId: string | null): boolean {
  if (parentId === null) return true
  if (parentId === id) return false
  const parent = nodes.find((n) => n.id === parentId)
  if (parent === undefined || parent.kind === 'database') return false
  return !descendantIds(nodes, id).includes(parentId)
}

/**
 * dropFor works out where a page dragged onto a row lands: on the row it becomes the last child,
 * above or below it the page stands next to it. It returns null when the drop is not allowed or
 * leaves the page where it is.
 */
export function dropFor(
  nodes: PageNode[],
  dragId: string,
  overId: string,
  zone: DropZone,
): Drop | null {
  const dragged = nodes.find((n) => n.id === dragId)
  const over = nodes.find((n) => n.id === overId)
  if (dragged === undefined || over === undefined || dragId === overId) return null

  let drop: Drop
  if (zone === 'on') {
    if (!canMoveInto(nodes, dragId, overId)) return null
    drop = {
      parentId: overId,
      index: childrenOf(nodes, overId).filter((n) => n.id !== dragId).length,
    }
  } else {
    const parentId = over.parentId
    if (!canMoveInto(nodes, dragId, parentId)) return null
    const siblings = childrenOf(nodes, parentId)
    const overAt = siblings.findIndex((n) => n.id === overId)
    const draggedAt = siblings.findIndex((n) => n.id === dragId)
    // The place counts the siblings without the page itself: a page that stood higher shifts the row up.
    const shift = draggedAt !== -1 && draggedAt < overAt ? 1 : 0
    drop = { parentId, index: overAt + (zone === 'after' ? 1 : 0) - shift }
  }

  const here = childrenOf(nodes, dragged.parentId).findIndex((n) => n.id === dragId)
  if (drop.parentId === dragged.parentId && drop.index === here) return null
  return drop
}

/**
 * parentCandidates lists the pages the page may be moved under, whose title contains the query
 * (case-insensitive), in the order of the tree.
 */
export function parentCandidates(nodes: PageNode[], id: string, query: string): PageNode[] {
  const q = query.trim().toLowerCase()
  const skip = new Set([id, ...descendantIds(nodes, id)])
  const order = new Map<string, number>()
  visibleRows(buildTree(nodes), new Set(nodes.map((n) => n.id))).forEach((r, i) => {
    order.set(r.node.id, i)
  })
  return nodes
    .filter(
      (n) =>
        n.kind === 'page' && !skip.has(n.id) && (q === '' || n.title.toLowerCase().includes(q)),
    )
    .sort((a, b) => (order.get(a.id) ?? 0) - (order.get(b.id) ?? 0))
}

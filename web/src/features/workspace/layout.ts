import type { SerializedDockview } from 'dockview-react'

export type Screen = 'table' | 'wiki'

export const LAYOUT_VERSION = 1

/** layoutKey is where the layout of an account in a campaign on a screen is kept on this device. */
export function layoutKey(accountId: string, campaignId: string, screen: Screen): string {
  return `dndvtt.layout.v${String(LAYOUT_VERSION)}.${accountId}.${campaignId}.${screen}`
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

/** loadLayout returns the saved layout, or null when there is none, storage is blocked, the JSON is broken or the version differs. */
export function loadLayout(key: string): SerializedDockview | null {
  try {
    const raw = window.localStorage.getItem(key)
    if (raw === null) return null
    const saved: unknown = JSON.parse(raw)
    if (!isRecord(saved) || saved.v !== LAYOUT_VERSION || !isRecord(saved.layout)) return null
    return saved.layout as unknown as SerializedDockview
  } catch {
    return null
  }
}

/** saveLayout keeps the layout; a blocked or full storage is ignored, the layout is a convenience. */
export function saveLayout(key: string, layout: SerializedDockview): void {
  try {
    window.localStorage.setItem(key, JSON.stringify({ v: LAYOUT_VERSION, layout }))
  } catch {
    // Storage may be blocked.
  }
}

/** clearLayout forgets the saved layout. */
export function clearLayout(key: string): void {
  try {
    window.localStorage.removeItem(key)
  } catch {
    // Storage may be blocked.
  }
}

interface GroupData {
  id: string
  views: string[]
  activeView?: string
}

interface GridNode {
  type: 'leaf' | 'branch'
  data: GroupData | GridNode[]
  size?: number
  visible?: boolean
}

function isGroup(data: GridNode['data']): data is GroupData {
  return !Array.isArray(data)
}

/** leaves lists the groups of a grid in order. */
function leaves(node: GridNode): GroupData[] {
  return isGroup(node.data) ? [node.data] : node.data.flatMap(leaves)
}

/** prune drops the groups without views and the branches left without children. */
function prune(node: GridNode): GridNode | null {
  if (isGroup(node.data)) return node.data.views.length > 0 ? node : null
  const children = node.data.map(prune).filter((n): n is GridNode => n !== null)
  return children.length > 0 ? { ...node, data: children } : null
}

/**
 * sanitizeLayout makes a saved layout fit the panels of this screen and role.
 * Panels of an unknown kind or not allowed (allowed(kind) === false) are dropped, as are the groups
 * they leave empty. The panels of popout windows come back to the main window, into the first group
 * without the canvas (the first group at all when every group holds it); floating groups are treated
 * the same way. It returns null when nothing usable is left.
 */
export function sanitizeLayout(
  layout: SerializedDockview,
  allowed: (kind: string) => boolean,
): SerializedDockview | null {
  const root = layout.grid.root as unknown as GridNode
  const panels = layout.panels
  const usable = (id: string) => {
    const kind = panels[id]?.contentComponent
    return kind !== undefined && allowed(kind)
  }

  // Copy the groups: the input is not changed.
  const copy = (node: GridNode): GridNode =>
    isGroup(node.data)
      ? { ...node, data: { ...node.data, views: node.data.views.filter(usable) } }
      : { ...node, data: node.data.map(copy) }
  const grid = prune(copy(root))
  if (grid === null) return null

  // The panels that lived outside the grid, in the order of the layout.
  const strays: string[] = []
  for (const float of layout.floatingGroups ?? []) {
    if (float.data !== undefined) strays.push(...float.data.views)
  }
  for (const popout of layout.popoutGroups ?? []) {
    if (popout.data !== undefined) strays.push(...popout.data.views)
    if (popout.grid !== undefined) {
      strays.push(...leaves(popout.grid.root as unknown as GridNode).flatMap((g) => g.views))
    }
  }
  const groups = leaves(grid)
  const target = groups.find((g) => !g.views.some((v) => panels[v]?.contentComponent === 'scene'))
  const home = target ?? groups[0]
  if (home === undefined) return null
  const placed = new Set(groups.flatMap((g) => g.views))
  for (const id of strays) {
    if (usable(id) && !placed.has(id)) {
      home.views.push(id)
      placed.add(id)
    }
  }

  for (const group of groups) {
    if (group.activeView === undefined || !group.views.includes(group.activeView)) {
      group.activeView = group.views[0]
    }
  }
  const kept: SerializedDockview['panels'] = {}
  for (const id of placed) {
    const state = panels[id]
    if (state !== undefined) kept[id] = state
  }
  const activeGroup =
    layout.activeGroup !== undefined && groups.some((g) => g.id === layout.activeGroup)
      ? layout.activeGroup
      : undefined
  return {
    grid: { ...layout.grid, root: grid },
    panels: kept,
    ...(activeGroup === undefined ? {} : { activeGroup }),
  }
}

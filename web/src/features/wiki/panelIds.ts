import type { PageNode } from '../../shared/api/wiki'

/** PAGE_PANEL_PREFIX starts the id of the panel of a wiki page: `page:<pageId>`. */
export const PAGE_PANEL_PREFIX = 'page:'

/** pageIdOfPanel returns the page a panel id belongs to, or null when it is not a page panel. */
export function pageIdOfPanel(panelId: string): string | null {
  return panelId.startsWith(PAGE_PANEL_PREFIX) ? panelId.slice(PAGE_PANEL_PREFIX.length) : null
}

/** pagePanelTitle is the icon and the name of a page for its tab; `fallback` stands for a page that is unknown or has no name. */
export function pagePanelTitle(tree: PageNode[] | null, pageId: unknown, fallback: string): string {
  const node = tree?.find((n) => n.id === pageId)
  if (node === undefined) return fallback
  return [node.icon, node.title === '' ? fallback : node.title].filter((s) => s !== '').join(' ')
}

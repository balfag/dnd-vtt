import { useStore } from 'zustand'
import { createStore, type StoreApi } from 'zustand/vanilla'
import type { PageNode } from '../../shared/api/wiki'
import { descendantIds } from './tree/tree'

export interface WikiState {
  /** tree is the page tree of the campaign; null until it is loaded. */
  tree: PageNode[] | null
  /** open lists the pages that have a panel, in the order they were opened. */
  open: string[]
  /** active is the page whose panel the user worked in last; the narrow screen shows it. */
  active: string | null
  /** toolsVersion grows when tags, templates or the trash may have changed; their sections read again. */
  toolsVersion: number
  setTree: (nodes: PageNode[]) => void
  /** setOpen mirrors the panels the workspace holds. */
  setOpen: (ids: string[]) => void
  openPage: (id: string) => void
  closePage: (id: string) => void
  setActive: (id: string | null) => void
  /** removePage drops a deleted page and everything below it from the tree and the open pages. */
  removePage: (id: string) => void
  /** retitle changes the name in the tree before the server has it. */
  retitle: (id: string, title: string, icon?: string) => void
  bumpTools: () => void
  reset: () => void
}

export function createWikiStore(): StoreApi<WikiState> {
  return createStore<WikiState>()((set) => ({
    tree: null,
    open: [],
    active: null,
    toolsVersion: 0,
    setTree: (nodes) => {
      set({ tree: nodes })
    },
    setOpen: (ids) => {
      set((s) => ({
        open: ids,
        active:
          s.active !== null && !ids.includes(s.active) ? (ids[ids.length - 1] ?? null) : s.active,
      }))
    },
    openPage: (id) => {
      set((s) => ({ open: s.open.includes(id) ? s.open : [...s.open, id], active: id }))
    },
    closePage: (id) => {
      set((s) => {
        const open = s.open.filter((p) => p !== id)
        return { open, active: s.active === id ? (open[open.length - 1] ?? null) : s.active }
      })
    },
    setActive: (id) => {
      set({ active: id })
    },
    removePage: (id) => {
      set((s) => {
        const gone = new Set([id, ...descendantIds(s.tree ?? [], id)])
        const open = s.open.filter((p) => !gone.has(p))
        return {
          tree: s.tree === null ? null : s.tree.filter((n) => !gone.has(n.id)),
          open,
          active:
            s.active !== null && gone.has(s.active) ? (open[open.length - 1] ?? null) : s.active,
        }
      })
    },
    retitle: (id, title, icon) => {
      set((s) => ({
        tree:
          s.tree === null
            ? null
            : s.tree.map((n) => (n.id === id ? { ...n, title, icon: icon ?? n.icon } : n)),
      }))
    },
    bumpTools: () => {
      set((s) => ({ toolsVersion: s.toolsVersion + 1 }))
    },
    reset: () => {
      set({ tree: null, open: [], active: null })
    },
  }))
}

export const wikiStore: StoreApi<WikiState> = createWikiStore()

export function useWiki<T>(selector: (s: WikiState) => T): T {
  return useStore(wikiStore, selector)
}

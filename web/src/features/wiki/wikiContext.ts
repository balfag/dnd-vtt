import { createContext, useContext } from 'react'

/** WikiActions are the things the panels of the wiki do through the screen around them. */
export interface WikiActions {
  campaignId: string
  /** reload reads the tree again. */
  reload: () => Promise<void>
  /** open shows a page: in a panel of the workspace, or on the page tab of a narrow screen. */
  open: (id: string, opts?: { newGroup?: boolean }) => void
  /** create makes an empty page under parentId (null: the root) and opens it. */
  create: (parentId: string | null) => Promise<void>
  rename: (id: string, title: string) => Promise<void>
  move: (id: string, parentId: string | null, index: number) => Promise<void>
  remove: (id: string) => Promise<void>
  /** notify tells the user something in a line over the workspace; text is an i18n key. */
  notify: (textKey: string) => void
}

export const WikiContext = createContext<WikiActions | null>(null)

export function useWikiActions(): WikiActions {
  const value = useContext(WikiContext)
  if (value === null) throw new Error('useWikiActions: no wiki screen around')
  return value
}

/** PageEditor is what an open page panel lets the rest of the screen do to the page. */
export interface PageEditor {
  setTitle: (title: string) => void
}

/** pageEditors holds the panel of each page that is open; a page has one panel, so one autosaver. */
export const pageEditors = new Map<string, PageEditor>()

import { createContext, useContext } from 'react'

/** HandoutSource is what the GM shows to the players: a fragment of a page, before mentions and rolls are turned into text. */
export interface HandoutSource {
  /** title is the name of the page the fragment comes from. */
  title: string
  /** doc is the fragment as a document of the wiki schema. */
  doc: unknown
  /** resolveLabel is the current name of a mention target, or null when it is not known. */
  resolveLabel: (kind: string, id: string) => string | null
}

/** TableLinks are the things a page can do to the game; only the table, with a game running, has them. */
export interface TableLinks {
  /** mention follows a click on a mention of a scene or a token. */
  mention: (kind: string, id: string) => void
  /** roll asks the server to roll a formula; label is the name of the page. */
  roll: (formula: string, visibility: 'gm' | 'public' | 'blind', label: string) => void
  /** showHandout opens the dialog that shows a fragment to the players. */
  showHandout: (source: HandoutSource) => void
}

/** WikiActions are the things the panels of the wiki do through the screen around them. */
export interface WikiActions {
  campaignId: string
  /** table is set on the table of a running game; the wiki screen has none. */
  table?: TableLinks
  /** reload reads the tree again. */
  reload: () => Promise<void>
  /** open shows a page: in a panel of the workspace, or on the page tab of a narrow screen. */
  open: (id: string, opts?: { newGroup?: boolean }) => void
  /** create makes a page under parentId (null: the root) and opens it; without a template it is empty. */
  create: (parentId: string | null, template?: NewPageTemplate) => Promise<void>
  rename: (id: string, title: string) => Promise<void>
  move: (id: string, parentId: string | null, index: number) => Promise<void>
  remove: (id: string) => Promise<void>
  /** saveAsTemplate keeps the icon and the text of a page as a template of its own. */
  saveAsTemplate: (id: string, title: string) => Promise<void>
  /** focusSearch puts the cursor into the search field of the tree. */
  focusSearch: () => void
  /** notify tells the user something in a line over the workspace; text is an i18n key. */
  notify: (textKey: string) => void
}

/**
 * NewPageTemplate is what a new page is made from: a built-in template or one of the GM's own;
 * database makes a database instead of a page, from a built-in database template (null: an empty one).
 */
export type NewPageTemplate = { builtin: string } | { custom: string } | { database: string | null }

/** searchField lets the rest of the screen reach the search field; the field of the tree sets it. */
export const searchField: { focus: (() => void) | null; pending: boolean } = {
  focus: null,
  pending: false,
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

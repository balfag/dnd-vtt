import type { SnippetPart } from '../../../shared/api/wiki'

export interface ParsedQuery {
  /** tag is the name after "#" in the first word, or null. */
  tag: string | null
  /** q is the rest of the query: the words to search. */
  q: string
}

/** parseQuery reads "#tag words": only the first word may name a tag. */
export function parseQuery(input: string): ParsedQuery {
  const text = input.trim()
  if (text.startsWith('#')) {
    const space = text.search(/\s/)
    const word = space === -1 ? text : text.slice(0, space)
    const name = word.slice(1)
    if (name !== '') {
      return { tag: name, q: space === -1 ? '' : text.slice(space).trim() }
    }
  }
  return { tag: null, q: text }
}

export interface Segment {
  text: string
  match: boolean
}

/** segments turns a snippet into plain text pieces for React to render; nothing here is HTML. */
export function segments(snippet: SnippetPart[]): Segment[] {
  return snippet
    .filter((p) => p.text !== '')
    .map((p) => ({ text: p.text, match: p.match === true }))
}

/** hitId is the id of the option number index in the list of results. */
export function hitId(listId: string, index: number): string {
  return `${listId}-${String(index)}`
}

/** hasWords tells whether the server has anything to look for: a letter or a digit. */
export function hasWords(q: string): boolean {
  return /[\p{L}\p{N}]/u.test(q)
}

/** suggestTags lists the known tags that start with the typed text and are not on the page yet. */
export function suggestTags(known: string[], taken: string[], typed: string): string[] {
  const prefix = typed.trim().toLowerCase()
  const have = new Set(taken.map((n) => n.toLowerCase()))
  return known.filter((n) => !have.has(n.toLowerCase()) && n.toLowerCase().startsWith(prefix))
}

/** findTag looks a tag up by name, ignoring case. */
export function findTag<T extends { name: string }>(tags: T[], name: string): T | undefined {
  const lower = name.toLowerCase()
  return tags.find((t) => t.name.toLowerCase() === lower)
}

/** isSearchShortcut tells Ctrl+K and Cmd+K, on any keyboard layout (the Russian one types \л\). */
export function isSearchShortcut(e: {
  key: string
  code: string
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
}): boolean {
  return (e.ctrlKey || e.metaKey) && !e.altKey && (e.code === 'KeyK' || e.key.toLowerCase() === 'k')
}

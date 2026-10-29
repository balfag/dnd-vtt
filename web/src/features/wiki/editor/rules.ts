// Value rules of the wiki schema v1 (docs/architecture/protocol.md, "Содержимое страницы").
// The server checks the same rules (internal/wiki/doc/schema.go) and answers 400 to a document that breaks them,
// so the editor never puts such a value into a document.

import { isValidFormula } from '../../../shared/dice/parse'

export const mentionKinds = ['page', 'scene', 'token'] as const
export type MentionKind = (typeof mentionKinds)[number]

const languageRe = /^[a-z0-9+#-]{1,20}$/
const ulidRe = /^[0-9A-HJKMNP-TV-Z]{26}$/
const controlOrSurrogateRe = /[\p{Cc}\p{Cs}]/u

export const defaultCalloutEmoji = '💡'
export const minColumnWidth = 40
export const maxColumnWidth = 2000

export function isMentionKind(s: unknown): s is MentionKind {
  return mentionKinds.some((k) => k === s)
}

export function isUlid(s: unknown): s is string {
  return typeof s === 'string' && ulidRe.test(s)
}

/** isValidEmoji: 1 to 8 code points without control characters (internal/wiki/doc ValidIcon). */
export function isValidEmoji(s: string): boolean {
  const n = Array.from(s).length
  return n >= 1 && n <= 8 && !controlOrSurrogateRe.test(s)
}

/** isValidLanguage: the code block language is empty or 1 to 20 characters of [a-z0-9+#-]. */
export function isValidLanguage(s: unknown): s is string {
  return typeof s === 'string' && languageRe.test(s)
}

/** isAllowedHref: links are http, https or mailto only. */
export function isAllowedHref(href: string): boolean {
  return /^(https?:|mailto:)/i.test(href.trim()) && Array.from(href).length <= 2000
}

/**
 * normalizeHref turns what a person typed into a link target: a bare address gets "https://".
 * It returns null when the result is not an allowed link.
 */
export function normalizeHref(input: string): string | null {
  const s = input.trim()
  if (s === '') {
    return null
  }
  const withScheme = /^[a-z][a-z0-9+.-]*:/i.test(s) ? s : `https://${s}`
  return isAllowedHref(withScheme) ? withScheme : null
}

/** clampColumnWidth keeps a resized column inside the range the server accepts. */
export function clampColumnWidth(width: number): number {
  return Math.min(maxColumnWidth, Math.max(minColumnWidth, Math.round(width)))
}

// "[[<formula>]]" at the end of the typed text.
export const rollInputRe = /\[\[([^[\]]*)\]\]$/

/**
 * rollFromInput decides whether typed text "[[<formula>]]" becomes an inline roll:
 * it returns the formula only when the server would accept it, otherwise null (the text stays as typed).
 */
export function rollFromInput(typed: string): string | null {
  const match = rollInputRe.exec(typed)
  const formula = match?.[1]?.trim()
  return formula !== undefined && formula !== '' && isValidFormula(formula) ? formula : null
}

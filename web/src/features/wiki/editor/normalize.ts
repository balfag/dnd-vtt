import { Extension } from '@tiptap/core'
import { Plugin, type EditorState, type Transaction } from '@tiptap/pm/state'
import { clampColumnWidth, isValidLanguage } from './rules'

// The column width is null or one integer 40..2000. It returns the value to put instead,
// or undefined when the attribute is fine (a pasted table may carry several widths or junk).
function fixedColwidth(value: unknown): number[] | null | undefined {
  if (value === null || value === undefined) {
    return undefined
  }
  const first: unknown = Array.isArray(value) ? (value as unknown[])[0] : undefined
  if (typeof first !== 'number' || !Number.isFinite(first)) {
    return null
  }
  const width = clampColumnWidth(first)
  return (value as unknown[]).length === 1 && width === first ? undefined : [width]
}

/**
 * fixAttrs repairs the attribute values that TipTap lets through but the server rejects:
 * a code block language outside [a-z0-9+#-]{1,20} (typed after ``` or pasted) is cleared,
 * and a table column width outside 40..2000 px (dragged or pasted) is brought back into the range.
 * It returns null when the document is fine.
 */
export function fixAttrs(state: EditorState): Transaction | null {
  const tr = state.tr
  state.doc.descendants((node, pos) => {
    const name = node.type.name
    if (name === 'codeBlock') {
      const language: unknown = node.attrs.language
      if (language !== null && language !== '' && !isValidLanguage(language)) {
        tr.setNodeMarkup(pos, undefined, { ...node.attrs, language: null })
      }
    } else if (name === 'tableCell' || name === 'tableHeader') {
      const fixed = fixedColwidth(node.attrs.colwidth)
      if (fixed !== undefined) {
        tr.setNodeMarkup(pos, undefined, { ...node.attrs, colwidth: fixed })
      }
    }
    // The text of a block is not looked into.
    return !node.isTextblock
  })
  return tr.docChanged ? tr : null
}

export const fixAttrsPlugin = new Plugin({
  appendTransaction: (transactions, _old, state) =>
    transactions.some((tr) => tr.docChanged) ? fixAttrs(state) : null,
})

/** NormalizeAttrs keeps attribute values inside the ranges of the wiki schema v1. */
export const NormalizeAttrs = Extension.create({
  name: 'normalizeAttrs',
  addProseMirrorPlugins() {
    return [fixAttrsPlugin]
  },
})

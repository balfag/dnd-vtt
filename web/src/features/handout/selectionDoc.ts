import type { Editor } from '@tiptap/core'

/**
 * selectionDoc returns the selected fragment of the editor as a document of the wiki schema.
 * A selection that cuts through a list or a table does not make a valid document by itself; then
 * the fragment is its text, one paragraph per line.
 */
export function selectionDoc(editor: Editor): unknown {
  const { state } = editor
  const slice = state.selection.content()
  const doc = { type: 'doc', content: slice.content.toJSON() as unknown[] | null }
  try {
    if (Array.isArray(doc.content) && doc.content.length > 0) {
      editor.schema.nodeFromJSON(doc).check()
      return doc
    }
  } catch {
    // Fall through to the text.
  }
  const { from, to } = state.selection
  const lines = state.doc
    .textBetween(from, to, '\n')
    .split('\n')
    .filter((line) => line !== '')
  return {
    type: 'doc',
    content:
      lines.length === 0
        ? [{ type: 'paragraph' }]
        : lines.map((text) => ({ type: 'paragraph', content: [{ type: 'text', text }] })),
  }
}

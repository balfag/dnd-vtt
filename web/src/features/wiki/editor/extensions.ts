import type { Extensions } from '@tiptap/core'
import type { MentionOptions } from '@tiptap/extension-mention'
import type { SuggestionOptions } from '@tiptap/suggestion'
import { Details, DetailsContent, DetailsSummary } from '@tiptap/extension-details'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'
import { Placeholder } from '@tiptap/extensions'
import StarterKit from '@tiptap/starter-kit'
import i18n from 'i18next'
import { Callout } from './Callout'
import type { HandlersRef, MentionItem } from './handlers'
import { InlineRoll } from './InlineRoll'
import { NormalizeAttrs } from './normalize'
import { isAllowedHref, minColumnWidth } from './rules'
import { WikiImage } from './WikiImage'
import { WikiMention } from './WikiMention'

// Cells are never merged (the server accepts colspan = rowspan = 1 only): whatever a pasted
// table says, both stay 1, and the merge commands do nothing.
const fixedSpan = { default: 1, parseHTML: () => 1, renderHTML: () => ({}) }

const WikiTable = Table.extend({
  addCommands() {
    return {
      ...this.parent?.(),
      mergeCells: () => () => false,
      splitCell: () => () => false,
      mergeOrSplit: () => () => false,
    }
  },
})

const WikiTableCell = TableCell.extend({
  addAttributes() {
    return { ...this.parent?.(), colspan: fixedSpan, rowspan: fixedSpan }
  },
})

const WikiTableHeader = TableHeader.extend({
  addAttributes() {
    return { ...this.parent?.(), colspan: fixedSpan, rowspan: fixedSpan }
  },
})

// The default button has an English label and no text; the triangle itself is drawn by CSS.
function renderDetailsToggle({
  element,
  isOpen,
}: {
  element: HTMLButtonElement
  isOpen: boolean
}): void {
  element.textContent = isOpen ? '▾' : '▸'
  element.setAttribute(
    'aria-label',
    i18n.t(isOpen ? 'wiki.editor.details.collapse' : 'wiki.editor.details.expand'),
  )
}

export interface ExtensionConfig {
  /** handlers are read at the moment of use, so the callbacks may change while the editor lives. */
  handlers: HandlersRef
  /** mentionSuggestion is the @ menu; without it the mention node is shown but cannot be inserted. */
  mentionSuggestion?: Omit<SuggestionOptions<MentionItem, MentionItem>, 'editor'>
  /** extra extensions without nodes of their own: the menus. */
  extra?: Extensions
}

/**
 * createExtensions returns the extensions of the wiki schema v1 (docs/architecture/protocol.md,
 * "Содержимое страницы"): exactly its nodes and marks, nothing more.
 */
export function createExtensions({
  handlers,
  mentionSuggestion = {},
  extra = [],
}: ExtensionConfig): Extensions {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: {
        openOnClick: false,
        autolink: true,
        defaultProtocol: 'https',
        isAllowedUri: (url) => isAllowedHref(url),
      },
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    WikiTable.configure({ resizable: true, cellMinWidth: minColumnWidth }),
    TableRow,
    WikiTableCell,
    WikiTableHeader,
    WikiImage,
    Details.configure({ persist: true, renderToggleButton: renderDetailsToggle }),
    DetailsSummary,
    DetailsContent,
    Callout,
    InlineRoll.configure({ handlers }),
    WikiMention.configure({
      handlers, // Mention types its menu for its own attributes; ours passes a MentionItem, which carries them.
      suggestion: mentionSuggestion as MentionOptions['suggestion'],
    }),
    Placeholder.configure({ placeholder: () => handlers.current.placeholder ?? '' }),
    NormalizeAttrs,
    ...extra,
  ]
}

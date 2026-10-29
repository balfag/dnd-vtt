import type { Editor, Range } from '@tiptap/core'

export interface SlashContext {
  editor: Editor
  /** range is the typed "/query"; every command deletes it first. */
  range: Range
  /** pickImage opens the file chooser and inserts the chosen image. */
  pickImage: () => void
}

export interface SlashItem {
  id: string
  /** English synonyms: the menu searches by the title in the interface language and by these. */
  keywords: string[]
  /** shortcut is the markdown shortcut shown on the right. */
  shortcut?: string
  /** needsUpload items are hidden when the editor cannot upload images. */
  needsUpload?: boolean
  run: (ctx: SlashContext) => void
}

const start = ({ editor, range }: SlashContext) => editor.chain().focus().deleteRange(range)

/** The items of the "/" menu in the order of the menu. The title of an item is the key wiki.editor.slash.<id>. */
export const slashItems: SlashItem[] = [
  { id: 'text', keywords: ['text', 'paragraph', 'p'], run: (c) => start(c).setParagraph().run() },
  {
    id: 'h1',
    keywords: ['heading 1', 'h1', 'title'],
    shortcut: '#',
    run: (c) => start(c).setNode('heading', { level: 1 }).run(),
  },
  {
    id: 'h2',
    keywords: ['heading 2', 'h2'],
    shortcut: '##',
    run: (c) => start(c).setNode('heading', { level: 2 }).run(),
  },
  {
    id: 'h3',
    keywords: ['heading 3', 'h3'],
    shortcut: '###',
    run: (c) => start(c).setNode('heading', { level: 3 }).run(),
  },
  {
    id: 'bulletList',
    keywords: ['bullet list', 'unordered', 'ul'],
    shortcut: '-',
    run: (c) => start(c).toggleBulletList().run(),
  },
  {
    id: 'orderedList',
    keywords: ['numbered list', 'ordered', 'ol'],
    shortcut: '1.',
    run: (c) => start(c).toggleOrderedList().run(),
  },
  {
    id: 'taskList',
    keywords: ['checklist', 'todo', 'task list'],
    shortcut: '[]',
    run: (c) => start(c).toggleTaskList().run(),
  },
  {
    id: 'blockquote',
    keywords: ['quote', 'blockquote'],
    shortcut: '>',
    run: (c) => start(c).toggleBlockquote().run(),
  },
  {
    id: 'callout',
    keywords: ['callout', 'note', 'aside'],
    run: (c) =>
      start(c)
        .insertContent({ type: 'callout', content: [{ type: 'paragraph' }] })
        .run(),
  },
  {
    id: 'horizontalRule',
    keywords: ['divider', 'rule', 'hr', 'line'],
    shortcut: '---',
    run: (c) => start(c).setHorizontalRule().run(),
  },
  {
    id: 'codeBlock',
    keywords: ['code', 'snippet'],
    shortcut: '```',
    run: (c) => start(c).setCodeBlock().run(),
  },
  {
    id: 'table',
    keywords: ['table', 'grid'],
    run: (c) => start(c).insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  {
    id: 'details',
    keywords: ['toggle', 'collapsible', 'details', 'spoiler'],
    run: (c) => start(c).setDetails().run(),
  },
  {
    id: 'image',
    keywords: ['image', 'picture', 'photo'],
    needsUpload: true,
    run: (c) => {
      start(c).run()
      c.pickImage()
    },
  },
  {
    id: 'mention',
    keywords: ['mention', 'link', 'page', 'at'],
    shortcut: '@',
    run: (c) => start(c).insertContent('@').run(),
  },
  {
    id: 'roll',
    keywords: ['roll', 'dice', 'formula'],
    shortcut: '[[',
    run: (c) => start(c).insertContent('[[').run(),
  },
]

/**
 * filterSlashItems returns the items that match the query by their title (translated by `title`)
 * or by an English keyword, in the order of the menu; items that need an upload are dropped without one.
 */
export function filterSlashItems(
  items: SlashItem[],
  query: string,
  title: (item: SlashItem) => string,
  canUpload: boolean,
): SlashItem[] {
  const q = query.trim().toLowerCase()
  return items.filter((item) => {
    if (item.needsUpload && !canUpload) {
      return false
    }
    return (
      q === '' || title(item).toLowerCase().includes(q) || item.keywords.some((k) => k.includes(q))
    )
  })
}

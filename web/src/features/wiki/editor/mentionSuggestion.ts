import { PluginKey } from '@tiptap/pm/state'
import type { SuggestionOptions } from '@tiptap/suggestion'
import i18n from 'i18next'
import type { HandlersRef, MentionItem } from './handlers'
import { mentionKinds } from './rules'
import { createMenuRenderer } from './suggestionRender'

const kindOrder = new Map<string, number>(mentionKinds.map((k, i) => [k, i]))

/** groupByKind orders the items by kind (pages, scenes, tokens) and keeps the server's order inside a kind. */
export function groupByKind(items: MentionItem[]): MentionItem[] {
  return items
    .map((item, i) => ({ item, i }))
    .sort(
      (a, b) =>
        (kindOrder.get(a.item.kind) ?? 99) - (kindOrder.get(b.item.kind) ?? 99) || a.i - b.i,
    )
    .map(({ item }) => item)
}

/** createMentionSuggestion is the "@" menu: the items come from the `mentions` callback of the editor. */
export function createMentionSuggestion(
  handlers: HandlersRef,
): Omit<SuggestionOptions<MentionItem, MentionItem>, 'editor'> {
  return {
    pluginKey: new PluginKey('mentionMenu'),
    char: '@',
    debounce: 120,
    // Without a source of items there is nothing to offer.
    allow: ({ state, range }) =>
      handlers.current.mentions !== undefined &&
      state.doc.resolve(range.from).parent.type.name !== 'codeBlock',
    items: async ({ query }) => groupByKind((await handlers.current.mentions?.(query)) ?? []),
    command: ({ editor, range, props }) => {
      editor
        .chain()
        .focus()
        .insertContentAt(range, [
          { type: 'mention', attrs: { kind: props.kind, id: props.id, label: props.label } },
          { type: 'text', text: ' ' },
        ])
        .run()
    },
    render: createMenuRenderer<MentionItem>({
      toEntry: (item) => ({
        key: `${item.kind}:${item.id}`,
        label: item.label,
        detail: item.detail === '' ? undefined : item.detail,
        group: i18n.t(`wiki.editor.mention.group.${item.kind}`),
      }),
      label: () => i18n.t('wiki.editor.mention.label'),
      emptyText: () => i18n.t('wiki.editor.mention.empty'),
    }),
  }
}

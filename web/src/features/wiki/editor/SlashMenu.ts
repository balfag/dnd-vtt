import { Extension, type Editor } from '@tiptap/core'
import { PluginKey } from '@tiptap/pm/state'
import Suggestion from '@tiptap/suggestion'
import i18n from 'i18next'
import { filterSlashItems, slashItems, type SlashItem } from './slashItems'
import { createMenuRenderer } from './suggestionRender'

export interface SlashMenuOptions {
  /** canUpload tells whether the "Image" item is offered. */
  canUpload: () => boolean
  pickImage: (editor: Editor) => void
}

const title = (item: SlashItem) => i18n.t(`wiki.editor.slash.${item.id}`)

/** SlashMenu opens the block menu on "/" (docs/design/wiki-screen.md, "Меню и состояния"). */
export const SlashMenu = Extension.create<SlashMenuOptions>({
  name: 'slashMenu',

  addOptions() {
    return { canUpload: () => false, pickImage: () => undefined }
  },

  addProseMirrorPlugins() {
    const { canUpload, pickImage } = this.options
    return [
      Suggestion<SlashItem, SlashItem>({
        editor: this.editor,
        pluginKey: new PluginKey('slashMenu'),
        char: '/',
        // Not inside a code block, where "/" is just a character.
        allow: ({ state, range }) => state.doc.resolve(range.from).parent.type.name !== 'codeBlock',
        items: ({ query }) => filterSlashItems(slashItems, query, title, canUpload()),
        command: ({ editor, range, props }) => {
          props.run({
            editor,
            range,
            pickImage: () => {
              pickImage(editor)
            },
          })
        },
        render: createMenuRenderer<SlashItem>({
          toEntry: (item) => ({ key: item.id, label: title(item), detail: item.shortcut }),
          label: () => i18n.t('wiki.editor.slash.label'),
          emptyText: () => i18n.t('wiki.editor.slash.empty'),
        }),
      }),
    ]
  },
})

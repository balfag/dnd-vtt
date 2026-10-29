import { ReactRenderer } from '@tiptap/react'
import type { SuggestionKeyDownProps, SuggestionOptions, SuggestionProps } from '@tiptap/suggestion'
import { SuggestionMenu, type MenuEntry, type SuggestionMenuProps } from './SuggestionMenu'

export interface MenuConfig<I> {
  toEntry: (item: I) => MenuEntry
  /** label is the accessible name of the list. */
  label: () => string
  emptyText: () => string
}

/**
 * createMenuRenderer makes the `render` option of a Suggestion: the list is a React component
 * mounted (and positioned under the cursor) by the plugin; ↑ ↓ Enter Esc are handled here.
 */
export function createMenuRenderer<I>(
  config: MenuConfig<I>,
): NonNullable<SuggestionOptions<I, I>['render']> {
  return () => {
    let renderer: ReactRenderer<unknown, SuggestionMenuProps> | null = null
    let unmount: (() => void) | null = null
    let selected = 0
    let current: SuggestionProps<I, I> | null = null

    const menuProps = (props: SuggestionProps<I, I>): SuggestionMenuProps => ({
      entries: props.items.map(config.toEntry),
      selected,
      loading: props.loading,
      emptyText: config.emptyText(),
      label: config.label(),
      onSelect: (index) => {
        const item = props.items[index]
        if (item !== undefined) {
          props.command(item)
        }
      },
    })

    const show = (props: SuggestionProps<I, I>) => {
      current = props
      selected = Math.min(selected, Math.max(0, props.items.length - 1))
      renderer?.updateProps(menuProps(props))
    }

    return {
      onStart: (props) => {
        selected = 0
        current = props
        renderer = new ReactRenderer(SuggestionMenu, {
          props: menuProps(props),
          editor: props.editor,
        })
        unmount = props.mount(renderer.element)
      },
      onUpdate: (props) => {
        // A new query starts from the top of the list.
        if (props.query !== current?.query) {
          selected = 0
        }
        show(props)
      },
      onKeyDown: ({ event }: SuggestionKeyDownProps) => {
        const props = current
        if (!props || props.items.length === 0) {
          return false
        }
        const n = props.items.length
        switch (event.key) {
          case 'ArrowDown':
            selected = (selected + 1) % n
            break
          case 'ArrowUp':
            selected = (selected + n - 1) % n
            break
          case 'Enter': {
            const item = props.items[selected]
            if (item !== undefined) {
              props.command(item)
            }
            return true
          }
          default:
            return false
        }
        show(props)
        return true
      },
      onExit: () => {
        unmount?.()
        unmount = null
        renderer?.destroy()
        renderer = null
        current = null
      },
    }
  }
}

import { Node, mergeAttributes } from '@tiptap/core'
import i18n from 'i18next'
import { defaultCalloutEmoji, isValidEmoji } from './rules'

/** Callout is a block with an emoji on a board-coloured background (docs/design/wiki-screen.md, "Выноска"). */
export const Callout = Node.create({
  name: 'callout',
  group: 'block',
  content: 'block+',
  defining: true,

  addAttributes() {
    return {
      emoji: {
        default: defaultCalloutEmoji,
        parseHTML: (el) => {
          const emoji = el.getAttribute('data-emoji') ?? ''
          return isValidEmoji(emoji) ? emoji : defaultCalloutEmoji
        },
        renderHTML: (attrs) => ({ 'data-emoji': String(attrs.emoji) }),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="callout"]' }]
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      'div',
      mergeAttributes({ 'data-type': 'callout' }, HTMLAttributes),
      [
        'span',
        { class: 'wiki-callout__emoji', contenteditable: 'false' },
        String(node.attrs.emoji),
      ],
      ['div', { class: 'wiki-callout__body' }, 0],
    ]
  },

  addNodeView() {
    return ({ node, editor, getPos }) => {
      let current = node
      const dom = document.createElement('div')
      dom.setAttribute('data-type', 'callout')
      dom.className = 'wiki-callout'

      const emojiButton = document.createElement('button')
      emojiButton.type = 'button'
      emojiButton.className = 'wiki-callout__emoji'
      emojiButton.contentEditable = 'false'
      const body = document.createElement('div')
      body.className = 'wiki-callout__body'
      dom.append(emojiButton, body)

      const paint = () => {
        emojiButton.textContent = String(current.attrs.emoji)
        emojiButton.disabled = !editor.isEditable
        emojiButton.setAttribute('aria-label', i18n.t('wiki.editor.callout.change'))
      }
      paint()

      const setEmoji = (emoji: string) => {
        const pos = getPos()
        if (pos === undefined || emoji === current.attrs.emoji) {
          return
        }
        editor.view.dispatch(
          editor.state.tr.setNodeMarkup(pos, undefined, { ...current.attrs, emoji }),
        )
      }

      emojiButton.addEventListener('click', () => {
        if (!editor.isEditable) {
          return
        }
        // The button turns into a small field; a value that breaks the rule leaves the old emoji.
        const input = document.createElement('input')
        input.type = 'text'
        input.className = 'wiki-callout__input'
        input.value = String(current.attrs.emoji)
        input.setAttribute('aria-label', i18n.t('wiki.editor.callout.change'))
        let done = false
        const finish = (apply: boolean) => {
          if (done) {
            return
          }
          done = true
          const value = input.value.trim()
          input.replaceWith(emojiButton)
          if (apply && isValidEmoji(value)) {
            setEmoji(value)
          }
          editor.commands.focus()
        }
        input.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            finish(true)
          } else if (e.key === 'Escape') {
            e.preventDefault()
            finish(false)
          }
        })
        input.addEventListener('blur', () => {
          finish(true)
        })
        emojiButton.replaceWith(input)
        input.focus()
        input.select()
      })

      return {
        dom,
        contentDOM: body,
        update: (updated) => {
          if (updated.type !== current.type) {
            return false
          }
          current = updated
          paint()
          return true
        },
        stopEvent: (e) =>
          e.target instanceof HTMLElement &&
          e.target.closest('.wiki-callout__emoji, .wiki-callout__input') !== null,
        ignoreMutation: (m) => !body.contains(m.target),
      }
    }
  },
})

import { mergeAttributes } from '@tiptap/core'
import Mention, { type MentionOptions } from '@tiptap/extension-mention'
import i18n from 'i18next'
import { HandlersRef } from './handlers'
import { isMentionKind, isUlid } from './rules'

export interface WikiMentionOptions {
  handlers: HandlersRef
}

function attr(el: HTMLElement, name: string): string {
  return el.getAttribute(`data-${name}`) ?? ''
}

/** WikiMention is an inline reference to a page, a scene or a token: attributes kind, id and label. */
export const WikiMention = Mention.extend<MentionOptions & WikiMentionOptions>({
  addOptions() {
    return { ...(this.parent?.() as MentionOptions), handlers: new HandlersRef() }
  },

  addAttributes() {
    return {
      kind: {
        default: 'page',
        isRequired: true,
        parseHTML: (el) => attr(el, 'kind'),
        renderHTML: (attrs) => ({ 'data-kind': String(attrs.kind) }),
      },
      id: {
        default: '',
        isRequired: true,
        parseHTML: (el) => attr(el, 'id'),
        renderHTML: (attrs) => ({ 'data-id': String(attrs.id) }),
      },
      label: {
        default: '',
        parseHTML: (el) => attr(el, 'label'),
        renderHTML: (attrs) => ({ 'data-label': String(attrs.label) }),
      },
    }
  },

  parseHTML() {
    return [
      {
        tag: 'span[data-type="mention"]',
        // A reference the server would reject is not a mention.
        getAttrs: (el) =>
          isMentionKind(attr(el, 'kind')) && isUlid(attr(el, 'id')) ? null : false,
      },
    ]
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      'span',
      mergeAttributes({ 'data-type': 'mention' }, HTMLAttributes),
      `@${String(node.attrs.label)}`,
    ]
  },

  renderText({ node }) {
    return `@${String(node.attrs.label)}`
  },

  addNodeView() {
    const { handlers } = this.options
    return ({ node }) => {
      let current = node
      const dom = document.createElement('span')
      dom.className = 'wiki-mention'
      dom.contentEditable = 'false'
      dom.tabIndex = 0
      dom.setAttribute('role', 'link')

      const paint = () => {
        const kind = String(current.attrs.kind)
        const id = String(current.attrs.id)
        const stored = String(current.attrs.label)
        const resolved = handlers.current.resolveLabel?.(kind, id)
        const gone = resolved === null
        const name = (resolved ?? stored) || i18n.t('wiki.editor.mention.unnamed')

        dom.replaceChildren()
        dom.classList.toggle('wiki-mention--gone', gone)
        dom.dataset.kind = kind
        if (kind === 'scene' || kind === 'token') {
          const tag = document.createElement('span')
          tag.className = 'wiki-mention__kind'
          tag.textContent = i18n.t(`wiki.editor.mention.kind.${kind}`)
          dom.append(tag, ' ')
        }
        const text = document.createElement('span')
        text.className = 'wiki-mention__name'
        text.textContent = gone ? `${name} ${i18n.t('wiki.editor.mention.deleted')}` : name
        dom.append(text)
        dom.setAttribute(
          'aria-label',
          i18n.t(`wiki.editor.mention.aria.${isMentionKind(kind) ? kind : 'page'}`, { name }),
        )
      }
      paint()

      const open = () => {
        handlers.current.onMentionClick?.(String(current.attrs.kind), String(current.attrs.id))
      }
      dom.addEventListener('click', (e) => {
        e.preventDefault()
        open()
      })
      dom.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          open()
        }
      })

      return {
        dom,
        update: (updated) => {
          if (updated.type !== current.type) {
            return false
          }
          current = updated
          paint()
          return true
        },
        stopEvent: (e) => e.type === 'mousedown' || e.type === 'click' || e.type === 'keydown',
      }
    }
  },
})

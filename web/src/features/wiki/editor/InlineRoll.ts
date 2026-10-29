import { InputRule, Node, mergeAttributes } from '@tiptap/core'
import i18n from 'i18next'
import { HandlersRef } from './handlers'
import { openPopupMenu } from './popupMenu'
import { rollFromInput, rollInputRe } from './rules'
import { isValidFormula } from '../../../shared/dice/parse'

export interface InlineRollOptions {
  handlers: HandlersRef
}

const svgNS = 'http://www.w3.org/2000/svg'

// A die: a rounded square with three pips.
function dieIcon(): SVGElement {
  const svg = document.createElementNS(svgNS, 'svg')
  svg.setAttribute('viewBox', '0 0 16 16')
  svg.setAttribute('width', '14')
  svg.setAttribute('height', '14')
  svg.setAttribute('aria-hidden', 'true')
  const body = document.createElementNS(svgNS, 'rect')
  for (const [k, v] of Object.entries({
    x: '1.5',
    y: '1.5',
    width: '13',
    height: '13',
    rx: '3',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '1.3',
  })) {
    body.setAttribute(k, v)
  }
  svg.append(body)
  for (const [cx, cy] of [
    ['5', '5'],
    ['8', '8'],
    ['11', '11'],
  ] as const) {
    const pip = document.createElementNS(svgNS, 'circle')
    pip.setAttribute('cx', cx)
    pip.setAttribute('cy', cy)
    pip.setAttribute('r', '1')
    pip.setAttribute('fill', 'currentColor')
    svg.append(pip)
  }
  return svg
}

/** InlineRoll is an atomic inline dice formula: a button that asks the server to roll it. */
export const InlineRoll = Node.create<InlineRollOptions>({
  name: 'inlineRoll',
  group: 'inline',
  inline: true,
  atom: true,

  addOptions() {
    return { handlers: new HandlersRef() }
  },

  addAttributes() {
    return {
      formula: {
        default: '',
        isRequired: true,
        parseHTML: (el) => el.getAttribute('data-formula') ?? '',
        renderHTML: (attrs) => ({ 'data-formula': String(attrs.formula) }),
      },
    }
  },

  parseHTML() {
    return [
      {
        tag: 'span[data-type="inline-roll"]',
        // A roll with a formula the server would reject is dropped; its text stays.
        getAttrs: (el) => (isValidFormula(el.getAttribute('data-formula') ?? '') ? null : false),
      },
    ]
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      'span',
      mergeAttributes({ 'data-type': 'inline-roll' }, HTMLAttributes),
      `[[${String(node.attrs.formula)}]]`,
    ]
  },

  renderText({ node }) {
    return `[[${String(node.attrs.formula)}]]`
  },

  addInputRules() {
    return [
      new InputRule({
        find: rollInputRe,
        handler: ({ state, range, match }) => {
          // The last typed "]" is not in the document yet; the match is the whole "[[...]]".
          const formula = rollFromInput(match[0])
          if (formula === null) {
            return null
          }
          state.tr.replaceWith(range.from, range.to, this.type.create({ formula }))
        },
      }),
    ]
  },

  addNodeView() {
    const { handlers } = this.options
    return ({ node }) => {
      let formula = String(node.attrs.formula)
      const dom = document.createElement('button')
      dom.type = 'button'
      dom.className = 'wiki-roll'
      dom.contentEditable = 'false'
      const label = document.createElement('span')
      dom.append(dieIcon(), label)

      const paint = () => {
        const canRoll = handlers.current.onRoll !== undefined
        label.textContent = formula
        dom.disabled = !canRoll
        dom.title = canRoll ? '' : i18n.t('wiki.editor.rollOffline')
        dom.setAttribute('aria-label', i18n.t('wiki.editor.rollAria', { formula }))
      }
      paint()

      dom.addEventListener('click', (e) => {
        e.preventDefault()
        handlers.current.onRoll?.(formula, 'gm')
      })
      dom.addEventListener('contextmenu', (e) => {
        const onRoll = handlers.current.onRoll
        if (!onRoll) {
          return
        }
        e.preventDefault()
        openPopupMenu(e.clientX, e.clientY, [
          {
            label: i18n.t('wiki.editor.rollPublic'),
            onSelect: () => {
              onRoll(formula, 'public')
            },
          },
          {
            label: i18n.t('wiki.editor.rollBlind'),
            onSelect: () => {
              onRoll(formula, 'blind')
            },
          },
        ])
      })

      return {
        dom,
        update: (updated) => {
          if (updated.type !== node.type) {
            return false
          }
          formula = String(updated.attrs.formula)
          paint()
          return true
        },
        stopEvent: (e) => e.type === 'mousedown' || e.type === 'click' || e.type === 'contextmenu',
      }
    }
  },
})

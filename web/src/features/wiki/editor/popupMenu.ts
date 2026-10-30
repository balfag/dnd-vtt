import { isNode } from '../../../shared/dom'

export interface PopupMenuItem {
  label: string
  onSelect: () => void
}

/**
 * openPopupMenu shows a small context menu at the point and closes it on a choice, Esc or a click outside.
 * doc is the document the menu opens in: a detached panel lives in a window of its own.
 */
export function openPopupMenu(doc: Document, x: number, y: number, items: PopupMenuItem[]): void {
  const menu = doc.createElement('div')
  menu.className = 'wiki-popup'
  menu.setAttribute('role', 'menu')
  menu.style.left = `${String(x)}px`
  menu.style.top = `${String(y)}px`

  const close = () => {
    menu.remove()
    doc.removeEventListener('mousedown', onOutside, true)
    doc.removeEventListener('keydown', onKey, true)
  }
  const onOutside = (e: Event) => {
    if (!isNode(e.target) || !menu.contains(e.target)) {
      close()
    }
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    }
  }

  for (const item of items) {
    const button = doc.createElement('button')
    button.type = 'button'
    button.className = 'wiki-popup__item'
    button.setAttribute('role', 'menuitem')
    button.textContent = item.label
    button.addEventListener('click', () => {
      close()
      item.onSelect()
    })
    menu.append(button)
  }

  doc.body.append(menu)
  doc.addEventListener('mousedown', onOutside, true)
  doc.addEventListener('keydown', onKey, true)
  menu.querySelector('button')?.focus()
}

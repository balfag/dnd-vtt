export interface PopupMenuItem {
  label: string
  onSelect: () => void
}

/** openPopupMenu shows a small context menu at the point and closes it on a choice, Esc or a click outside. */
export function openPopupMenu(x: number, y: number, items: PopupMenuItem[]): void {
  const menu = document.createElement('div')
  menu.className = 'wiki-popup'
  menu.setAttribute('role', 'menu')
  menu.style.left = `${String(x)}px`
  menu.style.top = `${String(y)}px`

  const close = () => {
    menu.remove()
    document.removeEventListener('mousedown', onOutside, true)
    document.removeEventListener('keydown', onKey, true)
  }
  const onOutside = (e: Event) => {
    if (!(e.target instanceof Node) || !menu.contains(e.target)) {
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
    const button = document.createElement('button')
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

  document.body.append(menu)
  document.addEventListener('mousedown', onOutside, true)
  document.addEventListener('keydown', onKey, true)
  menu.querySelector('button')?.focus()
}

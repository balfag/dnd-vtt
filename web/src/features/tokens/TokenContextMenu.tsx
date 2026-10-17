import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Token } from '../../shared/protocol'
import { tokenMenuItems, type TokenAction } from './tokenActions'
// The menu is drawn with the classes of the scene menu.
import '../scenes/scenes.css'
import './tokens.css'

interface TokenContextMenuProps {
  token: Token
  /** at is where the menu opens, in window coordinates. */
  at: { x: number; y: number }
  onAction: (action: TokenAction) => void
  /** onClose is called for Escape, a click away and Tab; the choice of an action closes the menu through onAction. */
  onClose: () => void
}

const edge = 8

/** TokenContextMenu is the menu of the right button on a token: the same actions as the keys. */
export function TokenContextMenu({ token, at, onAction, onClose }: TokenContextMenuProps) {
  const { t } = useTranslation()
  const menu = useRef<HTMLUListElement>(null)
  const [place, setPlace] = useState({ left: at.x, top: at.y })
  const items = tokenMenuItems(token)

  // Keep the whole menu in the window: it opens up or to the left near the edge.
  useLayoutEffect(() => {
    const rect = menu.current?.getBoundingClientRect()
    if (rect === undefined) return
    const left = Math.max(edge, Math.min(at.x, window.innerWidth - rect.width - edge))
    const top = Math.max(edge, Math.min(at.y, window.innerHeight - rect.height - edge))
    setPlace({ left, top })
  }, [at.x, at.y])

  useEffect(() => {
    menu.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [])

  useEffect(() => {
    const outside = (e: PointerEvent) => {
      if (e.target instanceof Node && !menu.current?.contains(e.target)) onClose()
    }
    document.addEventListener('pointerdown', outside)
    window.addEventListener('resize', onClose)
    return () => {
      document.removeEventListener('pointerdown', outside)
      window.removeEventListener('resize', onClose)
    }
  }, [onClose])

  function onKeyDown(e: React.KeyboardEvent) {
    const list = Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('button') ?? [])
    const at = list.findIndex((el) => el === document.activeElement)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const step = e.key === 'ArrowDown' ? 1 : -1
      list[(at + step + list.length) % list.length]?.focus()
    } else if (e.key === 'Home') {
      e.preventDefault()
      list[0]?.focus()
    } else if (e.key === 'End') {
      e.preventDefault()
      list[list.length - 1]?.focus()
    } else if (e.key === 'Escape' || e.key === 'Tab') {
      e.preventDefault()
      e.stopPropagation()
      onClose()
    }
  }

  return (
    <ul
      ref={menu}
      className="menu token-menu"
      role="menu"
      aria-label={t('tokens.menu.label')}
      style={place}
      onKeyDown={onKeyDown}
      onContextMenu={(e) => {
        e.preventDefault()
      }}
    >
      {items.map((item) => (
        <li key={item.action} role="none">
          <button
            type="button"
            role="menuitem"
            className={item.action === 'delete' ? 'menu__item menu__item--danger' : 'menu__item'}
            aria-keyshortcuts={item.shortcut}
            onClick={() => {
              onAction(item.action)
            }}
          >
            <span>{t(item.label)}</span>
            <kbd className="token-menu__key">{item.shortcut}</kbd>
          </button>
        </li>
      ))}
    </ul>
  )
}

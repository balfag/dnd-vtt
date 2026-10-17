import { useEffect, useId, useRef, useState } from 'react'

export interface CombatMenuItem {
  id: string
  label: string
  onSelect: () => void
  danger?: boolean
}

interface CombatMenuProps {
  /** label names the "⋯" button and the menu. */
  label: string
  items: CombatMenuItem[]
}

interface Place {
  /** Distance from the right edge of the window. */
  right: number
  top?: number
  bottom?: number
}

/** Room the menu needs under the button; with less it opens upwards. */
const menuHeight = 180

function MoreIcon() {
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false">
      <circle cx="5" cy="12" r="1.8" fill="currentColor" />
      <circle cx="12" cy="12" r="1.8" fill="currentColor" />
      <circle cx="19" cy="12" r="1.8" fill="currentColor" />
    </svg>
  )
}

/** CombatMenu is a "⋯" button and its menu of actions; the look of the menu is the scene menu's. */
export function CombatMenu({ label, items }: CombatMenuProps) {
  const [place, setPlace] = useState<Place | null>(null)
  const button = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLUListElement>(null)
  const menuId = useId()
  const open = place !== null

  function enabledItems(): HTMLButtonElement[] {
    return Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('button:enabled') ?? [])
  }

  function close(restoreFocus: boolean) {
    setPlace(null)
    if (restoreFocus) button.current?.focus()
  }

  function toggle() {
    if (open) {
      close(false)
      return
    }
    const rect = button.current?.getBoundingClientRect()
    if (rect === undefined) return
    const right = window.innerWidth - rect.right
    const room = window.innerHeight - rect.bottom
    setPlace(
      room >= menuHeight
        ? { right, top: rect.bottom + 4 }
        : { right, bottom: window.innerHeight - rect.top + 4 },
    )
  }

  // The first item gets the focus when the menu opens.
  useEffect(() => {
    if (open) menu.current?.querySelector<HTMLButtonElement>('button:enabled')?.focus()
  }, [open])

  // The menu is placed by the position of the button, so anything that moves it closes the menu.
  useEffect(() => {
    if (!open) return
    const outside = (e: PointerEvent) => {
      const target = e.target
      if (
        target instanceof Node &&
        !menu.current?.contains(target) &&
        !button.current?.contains(target)
      ) {
        setPlace(null)
      }
    }
    const away = () => {
      setPlace(null)
    }
    document.addEventListener('pointerdown', outside)
    window.addEventListener('resize', away)
    window.addEventListener('scroll', away, true)
    return () => {
      document.removeEventListener('pointerdown', outside)
      window.removeEventListener('resize', away)
      window.removeEventListener('scroll', away, true)
    }
  }, [open])

  function onKeyDown(e: React.KeyboardEvent) {
    const list = enabledItems()
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
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      close(true)
    } else if (e.key === 'Tab') {
      close(false)
    }
  }

  return (
    <>
      <button
        ref={button}
        type="button"
        className="combat__more"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={toggle}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && !open) {
            e.preventDefault()
            toggle()
          }
        }}
      >
        <MoreIcon />
      </button>
      {open && (
        <ul
          ref={menu}
          id={menuId}
          className="menu"
          role="menu"
          aria-label={label}
          style={place}
          onKeyDown={onKeyDown}
        >
          {items.map((item) => (
            <li key={item.id} role="none">
              <button
                type="button"
                role="menuitem"
                className={item.danger === true ? 'menu__item menu__item--danger' : 'menu__item'}
                onClick={() => {
                  close(true)
                  item.onSelect()
                }}
              >
                {item.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

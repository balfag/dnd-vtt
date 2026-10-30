import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { docOf, isNode } from '../../shared/dom'

export type MenuAction = 'rename' | 'settings' | 'moveUp' | 'moveDown' | 'delete'

interface SceneMenuProps {
  sceneName: string
  /** canMoveUp and canMoveDown are false for the first and the last scene. */
  canMoveUp: boolean
  canMoveDown: boolean
  /** isActive: players see the scene now, so it cannot be deleted. */
  isActive: boolean
  onAction: (action: MenuAction) => void
  /** buttonRef gives the row the button, to return the focus to it. */
  buttonRef: (el: HTMLButtonElement | null) => void
}

interface Place {
  /** Distance from the right edge of the window. */
  right: number
  top?: number
  bottom?: number
}

/** Room the menu needs under the button; with less it opens upwards. */
const menuHeight = 240

function MoreIcon() {
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false">
      <circle cx="5" cy="12" r="1.8" fill="currentColor" />
      <circle cx="12" cy="12" r="1.8" fill="currentColor" />
      <circle cx="19" cy="12" r="1.8" fill="currentColor" />
    </svg>
  )
}

/** SceneMenu is the "⋯" button of a row and its menu of actions. */
export function SceneMenu({
  sceneName,
  canMoveUp,
  canMoveDown,
  isActive,
  onAction,
  buttonRef,
}: SceneMenuProps) {
  const { t } = useTranslation()
  const [place, setPlace] = useState<Place | null>(null)
  const button = useRef<HTMLButtonElement | null>(null)
  const menu = useRef<HTMLUListElement>(null)
  const menuId = useId()
  const hintId = useId()
  const open = place !== null

  function setButton(el: HTMLButtonElement | null) {
    button.current = el
    buttonRef(el)
  }

  function items(): HTMLButtonElement[] {
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
    const win = docOf(button.current).defaultView ?? window
    const right = win.innerWidth - rect.right
    const room = win.innerHeight - rect.bottom
    setPlace(
      room >= menuHeight
        ? { right, top: rect.bottom + 4 }
        : { right, bottom: win.innerHeight - rect.top + 4 },
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
      if (isNode(target) && !menu.current?.contains(target) && !button.current?.contains(target)) {
        setPlace(null)
      }
    }
    const away = () => {
      setPlace(null)
    }
    const doc = docOf(button.current)
    const win = doc.defaultView ?? window
    doc.addEventListener('pointerdown', outside)
    win.addEventListener('resize', away)
    win.addEventListener('scroll', away, true)
    return () => {
      doc.removeEventListener('pointerdown', outside)
      win.removeEventListener('resize', away)
      win.removeEventListener('scroll', away, true)
    }
  }, [open])

  function onKeyDown(e: React.KeyboardEvent) {
    const list = items()
    const at = list.findIndex((el) => el === docOf(menu.current).activeElement)
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

  function choose(action: MenuAction) {
    close(false)
    onAction(action)
  }

  return (
    <>
      <button
        ref={setButton}
        type="button"
        className="scene-row__more"
        aria-label={t('scenes.menu', { name: sceneName })}
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
          aria-label={t('scenes.menu', { name: sceneName })}
          style={place}
          onKeyDown={onKeyDown}
        >
          <li role="none">
            <button
              type="button"
              role="menuitem"
              className="menu__item"
              onClick={() => {
                choose('rename')
              }}
            >
              {t('scenes.rename')}
            </button>
          </li>
          <li role="none">
            <button
              type="button"
              role="menuitem"
              className="menu__item"
              onClick={() => {
                choose('settings')
              }}
            >
              {t('scenes.settings')}
            </button>
          </li>
          <li role="none">
            <button
              type="button"
              role="menuitem"
              className="menu__item"
              disabled={!canMoveUp}
              onClick={() => {
                choose('moveUp')
              }}
            >
              {t('scenes.moveUp')}
            </button>
          </li>
          <li role="none">
            <button
              type="button"
              role="menuitem"
              className="menu__item"
              disabled={!canMoveDown}
              onClick={() => {
                choose('moveDown')
              }}
            >
              {t('scenes.moveDown')}
            </button>
          </li>
          <li role="none">
            <button
              type="button"
              role="menuitem"
              className="menu__item menu__item--danger"
              aria-disabled={isActive}
              aria-describedby={isActive ? hintId : undefined}
              onClick={() => {
                if (!isActive) choose('delete')
              }}
            >
              {t('scenes.delete')}
            </button>
            {isActive && (
              <p id={hintId} className="menu__hint">
                {t('scenes.deleteActive')}
              </p>
            )}
          </li>
        </ul>
      )}
    </>
  )
}

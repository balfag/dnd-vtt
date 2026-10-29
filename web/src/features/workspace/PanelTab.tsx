import type { IDockviewPanelHeaderProps } from 'dockview-react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { findPanel, paramsOf, useWorkspace } from './panels'

export interface MenuItem {
  key: string
  label: string
  onSelect: () => void
}

interface PopupMenuProps {
  /** doc is the document the menu opens in: a detached panel lives in a window of its own. */
  doc: Document
  /** at is the corner of the menu in the viewport; with align "end" it is the right-hand corner. */
  at: { x: number; y: number; align: 'start' | 'end' }
  label: string
  items: MenuItem[]
  /** onClose is called when the menu closes; restoreFocus is false when the click went elsewhere. */
  onClose: (restoreFocus: boolean) => void
}

/** PopupMenu is a small keyboard-driven menu over the workspace: arrows, Enter, Escape. */
export function PopupMenu({ doc, at, label, items, onClose }: PopupMenuProps) {
  const ref = useRef<HTMLDivElement>(null)
  // The parent makes a new onClose on every render; the menu must not re-focus because of it.
  const close = useRef(onClose)
  useEffect(() => {
    close.current = onClose
  })

  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
    const onPointerDown = (e: Event) => {
      if (e.target instanceof Node && ref.current?.contains(e.target) === true) return
      close.current(false)
    }
    doc.addEventListener('pointerdown', onPointerDown, true)
    return () => {
      doc.removeEventListener('pointerdown', onPointerDown, true)
    }
  }, [doc])

  function onKeyDown(e: React.KeyboardEvent) {
    const buttons = [...(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    const current = buttons.indexOf(doc.activeElement as HTMLElement)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const step = e.key === 'ArrowDown' ? 1 : -1
      buttons[(current + step + buttons.length) % buttons.length]?.focus()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose(true)
    } else if (e.key === 'Tab') {
      onClose(false)
    }
  }

  const style: React.CSSProperties =
    at.align === 'end'
      ? { top: at.y, right: Math.max(0, doc.documentElement.clientWidth - at.x) }
      : { top: at.y, left: at.x }
  return createPortal(
    <div
      ref={ref}
      className="wmenu"
      role="menu"
      aria-label={label}
      style={style}
      onKeyDown={onKeyDown}
    >
      {items.map((item) => (
        <button
          key={item.key}
          type="button"
          role="menuitem"
          className="wmenu__item"
          onClick={() => {
            onClose(true)
            item.onSelect()
          }}
        >
          {item.label}
        </button>
      ))}
    </div>,
    doc.body,
  )
}

/** PanelTab is the tab of a panel: its name, the menu "⋯" and "✕"; dockview supplies role="tab" and the name around it. */
export function PanelTab({ api, containerApi, params }: IDockviewPanelHeaderProps) {
  const { t } = useTranslation()
  const { panels, popout } = useWorkspace()
  const def = findPanel(panels, api.id)
  const menuButton = useRef<HTMLButtonElement>(null)
  const [maximized, setMaximized] = useState(() => api.isMaximized())
  const [menu, setMenu] = useState<{
    x: number
    y: number
    align: 'start' | 'end'
    doc: Document
  } | null>(null)

  useEffect(() => {
    const sub = containerApi.onDidMaximizedGroupChange(() => {
      setMaximized(api.isMaximized())
    })
    return () => {
      sub.dispose()
    }
  }, [api, containerApi])

  if (def === undefined) return null

  const items: MenuItem[] = []
  if (def.popout) {
    items.push({
      key: 'popout',
      label: t('workspace.popout'),
      onSelect: () => {
        popout(api.id)
      },
    })
  }
  items.push({
    key: 'maximize',
    label: maximized ? t('workspace.restore') : t('workspace.maximize'),
    onSelect: toggleMaximize,
  })
  if (def.closable) {
    items.push({
      key: 'close',
      label: t('workspace.close'),
      onSelect: () => {
        api.close()
      },
    })
  }

  function toggleMaximize() {
    if (api.isMaximized()) api.exitMaximized()
    else api.maximize()
  }

  function closeMenu(restoreFocus: boolean) {
    setMenu(null)
    if (restoreFocus) menuButton.current?.focus()
  }

  return (
    <div
      className="wtab"
      onDoubleClick={(e) => {
        if (e.target instanceof Element && e.target.closest('button') !== null) return
        toggleMaximize()
      }}
      onContextMenu={(e) => {
        e.preventDefault()
        e.stopPropagation()
        setMenu({ x: e.clientX, y: e.clientY, align: 'start', doc: e.currentTarget.ownerDocument })
      }}
    >
      <span className="wtab__title">{def.titleFor?.(paramsOf(params)) ?? def.title}</span>
      <button
        ref={menuButton}
        type="button"
        className="wtab__button"
        aria-label={t('workspace.menu')}
        aria-haspopup="menu"
        aria-expanded={menu !== null}
        draggable={false}
        onClick={() => {
          const el = menuButton.current
          if (el === null) return
          const rect = el.getBoundingClientRect()
          setMenu((now) =>
            now === null
              ? { x: rect.right, y: rect.bottom + 4, align: 'end', doc: el.ownerDocument }
              : null,
          )
        }}
      >
        <span aria-hidden="true">⋯</span>
      </button>
      {def.closable && (
        <button
          type="button"
          className="wtab__button"
          aria-label={t('workspace.close')}
          draggable={false}
          onClick={() => {
            api.close()
          }}
        >
          <span aria-hidden="true">✕</span>
        </button>
      )}
      {menu !== null && (
        <PopupMenu
          doc={menu.doc}
          at={menu}
          label={t('workspace.menu')}
          items={items}
          onClose={closeMenu}
        />
      )}
    </div>
  )
}

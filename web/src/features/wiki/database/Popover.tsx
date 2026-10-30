import { useEffect, useMemo, useRef } from 'react'
import { createPortal } from 'react-dom'
import { isElement } from '../../../shared/dom'

interface PopoverProps {
  anchor: HTMLElement
  label: string
  onClose: () => void
  /** align "end" puts the right edge of the panel under the right edge of the anchor. */
  align?: 'start' | 'end'
  children: React.ReactNode
}

/** Popover is a panel under a button: it closes on Escape and on a click elsewhere (a dialog opened from it does not count). */
export function Popover({ anchor, label, onClose, align = 'start', children }: PopoverProps) {
  const doc = anchor.ownerDocument
  const ref = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  useEffect(() => {
    close.current = onClose
  })

  // The anchor is on the screen when the panel opens, so its place is known at the first drawing.
  const style = useMemo<React.CSSProperties>(() => {
    const rect = anchor.getBoundingClientRect()
    const view = doc.documentElement
    // The panel is 34rem wide at most (database.css) and never wider than the window.
    const width = Math.min(544, view.clientWidth - 16)
    return align === 'end'
      ? { top: rect.bottom + 4, right: Math.max(8, view.clientWidth - rect.right) }
      : {
          top: rect.bottom + 4,
          left: Math.max(8, Math.min(rect.left, view.clientWidth - width - 8)),
        }
  }, [anchor, doc, align])

  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('input, select, button')?.focus()
    const onDown = (e: Event) => {
      if (!isElement(e.target)) return
      if (ref.current?.contains(e.target) === true || anchor.contains(e.target)) return
      if (e.target.closest('dialog, .dbpick') !== null) return
      close.current()
    }
    doc.addEventListener('pointerdown', onDown, true)
    return () => {
      doc.removeEventListener('pointerdown', onDown, true)
    }
  }, [doc, anchor])

  return createPortal(
    <div
      ref={ref}
      className="dbpop"
      role="dialog"
      aria-label={label}
      style={style}
      onKeyDown={(e) => {
        if (
          e.key === 'Escape' &&
          !e.defaultPrevented &&
          !(isElement(e.target) && e.target.closest('dialog') !== null)
        ) {
          e.preventDefault()
          e.stopPropagation()
          onClose()
        }
      }}
    >
      {children}
    </div>,
    doc.body,
  )
}

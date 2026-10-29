import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

export interface PickerItem {
  id: string
  label: React.ReactNode
  /** text is what the search matches and the screen readers read. */
  text: string
  selected: boolean
}

interface PickerPopupProps {
  /** anchor is the cell the list opens under. */
  anchor: HTMLElement
  label: string
  items: PickerItem[]
  loading?: boolean
  /** query and onQuery make the search field a controlled one; items are already filtered by the caller. */
  query: string
  onQuery: (q: string) => void
  /** onPick toggles an item; a single choice closes the list itself. */
  onPick: (id: string) => void
  /** onCreate offers "Create «query»" when the text does not name an item. */
  onCreate?: (name: string) => void
  /** full stops the choice of one more item. */
  full?: boolean
  onClose: () => void
}

const LIST_HEIGHT = 280

/** PickerPopup is a list with a search field under a cell: ↑ ↓ Enter, Escape. */
export function PickerPopup(p: PickerPopupProps) {
  const { t } = useTranslation()
  const doc = p.anchor.ownerDocument
  const ref = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const [active, setActive] = useState(0)
  const listId = useId()
  const close = useRef(p.onClose)
  useEffect(() => {
    close.current = p.onClose
  })

  const typed = p.query.trim()
  const canCreate =
    p.onCreate !== undefined &&
    typed !== '' &&
    !p.items.some((i) => i.text.toLocaleLowerCase() === typed.toLocaleLowerCase())
  const rows = p.items.length + (canCreate ? 1 : 0)
  const at = Math.min(active, Math.max(rows - 1, 0))

  // The cell is on the screen when the list opens, so its place is known at the first drawing.
  const style = useMemo<React.CSSProperties>(() => {
    const rect = p.anchor.getBoundingClientRect()
    const view = doc.documentElement
    const below = view.clientHeight - rect.bottom
    const flip = below < LIST_HEIGHT && rect.top > below
    return {
      left: Math.min(rect.left, Math.max(0, view.clientWidth - 240)),
      minWidth: Math.max(rect.width, 220),
      ...(flip ? { bottom: view.clientHeight - rect.top + 2 } : { top: rect.bottom + 2 }),
    }
  }, [p.anchor, doc])

  useEffect(() => {
    input.current?.focus()
    const onDown = (e: Event) => {
      if (!(e.target instanceof Node)) return
      if (ref.current?.contains(e.target) === true || p.anchor.contains(e.target)) return
      close.current()
    }
    // A second click on the cell (a double click) would move the focus from the search field to the cell.
    const keepFocus = (e: Event) => {
      if (e.target instanceof Node && p.anchor.contains(e.target)) e.preventDefault()
    }
    doc.addEventListener('pointerdown', onDown, true)
    doc.addEventListener('mousedown', keepFocus, true)
    return () => {
      doc.removeEventListener('pointerdown', onDown, true)
      doc.removeEventListener('mousedown', keepFocus, true)
    }
  }, [doc, p.anchor])

  function choose(index: number) {
    const item = p.items[index]
    if (item !== undefined) {
      if (item.selected || p.full !== true) p.onPick(item.id)
    } else if (canCreate && p.onCreate !== undefined) {
      p.onCreate(typed)
    }
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (rows > 0) setActive((at + (e.key === 'ArrowDown' ? 1 : -1) + rows) % rows)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      choose(at)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      p.onClose()
    } else if (e.key === 'Tab') {
      p.onClose()
    }
  }

  const optionId = (i: number) => `${listId}-${String(i)}`

  return createPortal(
    <div ref={ref} className="dbpick" style={style} onKeyDown={onKeyDown}>
      <input
        ref={input}
        className="dbpick__search"
        role="combobox"
        aria-expanded="true"
        aria-controls={listId}
        aria-activedescendant={rows > 0 ? optionId(at) : undefined}
        aria-label={p.label}
        placeholder={t(
          p.onCreate === undefined ? 'wiki.db.pick.searchOnly' : 'wiki.db.pick.search',
        )}
        value={p.query}
        onChange={(e) => {
          p.onQuery(e.target.value)
          setActive(0)
        }}
      />
      <ul
        id={listId}
        className="dbpick__list"
        role="listbox"
        aria-label={p.label}
        aria-multiselectable
      >
        {p.items.map((item, i) => (
          <li
            key={item.id}
            id={optionId(i)}
            role="option"
            aria-selected={item.selected}
            aria-disabled={!item.selected && p.full === true ? true : undefined}
            className={i === at ? 'dbpick__item dbpick__item--active' : 'dbpick__item'}
            onMouseEnter={() => {
              setActive(i)
            }}
            onClick={() => {
              choose(i)
            }}
          >
            <span className="dbpick__check" aria-hidden="true">
              {item.selected ? '✓' : ''}
            </span>
            {item.label}
          </li>
        ))}
        {canCreate && (
          <li
            id={optionId(p.items.length)}
            role="option"
            aria-selected={false}
            className={
              at === p.items.length
                ? 'dbpick__item dbpick__item--active dbpick__item--create'
                : 'dbpick__item dbpick__item--create'
            }
            onMouseEnter={() => {
              setActive(p.items.length)
            }}
            onClick={() => {
              choose(p.items.length)
            }}
          >
            {t('wiki.db.pick.create', { name: typed })}
          </li>
        )}
        {rows === 0 && (
          <li className="dbpick__note" role="presentation">
            {p.loading === true ? t('wiki.loading') : t('wiki.db.pick.empty')}
          </li>
        )}
      </ul>
      {p.full === true && <p className="dbpick__note">{t('wiki.db.pick.full')}</p>}
    </div>,
    doc.body,
  )
}

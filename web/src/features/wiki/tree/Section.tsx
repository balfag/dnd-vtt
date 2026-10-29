import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PopupMenu, type MenuItem } from '../../workspace/PanelTab'
import { RenameField } from './TreeRow'

interface SectionProps {
  name: string
  /** count is shown at the right of the heading. */
  count: number | null
  children: React.ReactNode
}

/** Section is a fold under the tree ("Tags", "Templates", "Trash"); it starts folded. */
export function Section({ name, count, children }: SectionProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const bodyId = useId()
  return (
    <section className="tsec">
      <h3 className="tsec__heading">
        <button
          type="button"
          className="tsec__toggle"
          aria-expanded={open}
          aria-controls={bodyId}
          aria-label={t(open ? 'wiki.tools.section.collapse' : 'wiki.tools.section.expand', {
            name,
          })}
          onClick={() => {
            setOpen((now) => !now)
          }}
        >
          <span className="tsec__arrow" aria-hidden="true">
            {open ? '▾' : '▸'}
          </span>
          <span className="tsec__name">{name}</span>
          {count !== null && count > 0 && <span className="tsec__count">{count}</span>}
        </button>
      </h3>
      <div id={bodyId} className="tsec__body" hidden={!open}>
        {children}
      </div>
    </section>
  )
}

interface SectionRowProps {
  icon?: string
  label: string
  /** meta is a quiet second line. */
  meta?: string
  /** count is the number at the right of the row. */
  count?: number
  /** menuLabel names the "⋯" menu for screen readers. */
  menuLabel: string
  items: MenuItem[]
  /** extra is a button or two between the name and the menu. */
  extra?: React.ReactNode
  onClick?: () => void
  renaming?: boolean
  renameLabel?: string
  renameMax?: number
  onRename?: (name: string) => void
  onCancelRename?: () => void
}

/** SectionRow is one line of a section: the name, what it says about itself, and a menu "⋯". */
export function SectionRow(p: SectionRowProps) {
  const [menu, setMenu] = useState<{
    x: number
    y: number
    align: 'start' | 'end'
    doc: Document
  } | null>(null)

  return (
    <li
      className="srow"
      onContextMenu={(e) => {
        e.preventDefault()
        setMenu({ x: e.clientX, y: e.clientY, align: 'start', doc: e.currentTarget.ownerDocument })
      }}
    >
      {p.icon !== undefined && (
        <span className="srow__icon" aria-hidden="true">
          {p.icon !== '' ? p.icon : '◦'}
        </span>
      )}
      {p.renaming === true ? (
        <RenameField
          label={p.renameLabel ?? p.label}
          initial={p.label}
          maxLength={p.renameMax}
          onCommit={(name) => p.onRename?.(name)}
          onCancel={() => p.onCancelRename?.()}
        />
      ) : (
        <button
          type="button"
          className="srow__main"
          onClick={p.onClick}
          disabled={p.onClick === undefined}
        >
          <span className="srow__name">{p.label}</span>
          {p.meta !== undefined && <span className="srow__meta">{p.meta}</span>}
        </button>
      )}
      {p.count !== undefined && p.renaming !== true && (
        <span className="srow__count">{p.count}</span>
      )}
      {p.renaming !== true && p.extra}
      {p.renaming !== true && (
        <button
          type="button"
          className="trow__button srow__menu"
          aria-label={p.menuLabel}
          aria-haspopup="menu"
          aria-expanded={menu !== null}
          onClick={(e) => {
            const rect = e.currentTarget.getBoundingClientRect()
            setMenu({
              x: rect.right,
              y: rect.bottom + 4,
              align: 'end',
              doc: e.currentTarget.ownerDocument,
            })
          }}
        >
          <span aria-hidden="true">⋯</span>
        </button>
      )}
      {menu !== null && (
        <PopupMenu
          doc={menu.doc}
          at={menu}
          label={p.menuLabel}
          items={p.items}
          onClose={() => {
            setMenu(null)
          }}
        />
      )}
    </li>
  )
}

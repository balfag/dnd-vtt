import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { PageNode } from '../../../shared/api/wiki'
import { PopupMenu, type MenuItem } from '../../workspace/PanelTab'
import type { DropZone } from './tree'

export interface TreeRowProps {
  node: PageNode
  depth: number
  hasChildren: boolean
  expanded: boolean
  /** selected marks the page shown in the panel the user worked in last. */
  selected: boolean
  /** tabbable gives the row the one tab stop of the tree. */
  tabbable: boolean
  /** level, size and index describe the row among its siblings for screen readers. */
  siblings: { size: number; index: number }
  renaming: boolean
  /** mark shows where a dragged page would land relative to this row. */
  mark: DropZone | null
  /** dragging is true while this row is the one being dragged. */
  dragging: boolean
  onToggle: () => void
  onOpen: (newGroup: boolean) => void
  onAddChild: () => void
  onStartRename: () => void
  onRename: (title: string) => void
  onCancelRename: () => void
  onMove: () => void
  onDelete: () => void
  onFocusRow: () => void
  onDragStart: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDrop: (e: React.DragEvent) => void
  onDragEnd: () => void
  registerRow: (el: HTMLDivElement | null) => void
}

const maxTitleLength = 200

/** TreeRow is one line of the page tree: the arrow, the icon, the name, and the buttons "+" and "⋯". */
export function TreeRow(p: TreeRowProps) {
  const { t } = useTranslation()
  const { node } = p
  const isDatabase = node.kind === 'database'
  const name = node.title === '' ? t('wiki.untitled') : node.title
  const nameId = useId()
  const menuButton = useRef<HTMLButtonElement>(null)
  const [menu, setMenu] = useState<{
    x: number
    y: number
    align: 'start' | 'end'
    doc: Document
  } | null>(null)

  const items: MenuItem[] = []
  if (!isDatabase) {
    items.push({ key: 'child', label: t('wiki.tree.addChild'), onSelect: p.onAddChild })
  }
  items.push(
    { key: 'rename', label: t('wiki.tree.rename'), onSelect: p.onStartRename },
    { key: 'move', label: t('wiki.tree.move'), onSelect: p.onMove },
  )
  if (!isDatabase) {
    items.push({
      key: 'group',
      label: t('wiki.tree.openNewGroup'),
      onSelect: () => {
        p.onOpen(true)
      },
    })
  }
  items.push({ key: 'delete', label: t('wiki.tree.delete'), onSelect: p.onDelete })

  function openMenuAt(el: HTMLElement) {
    const rect = el.getBoundingClientRect()
    setMenu({ x: rect.right, y: rect.bottom + 4, align: 'end', doc: el.ownerDocument })
  }

  const classes = [
    'trow',
    p.selected ? 'trow--selected' : '',
    p.dragging ? 'trow--dragging' : '',
    p.mark === null ? '' : `trow--drop-${p.mark}`,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div
      ref={(el) => {
        p.registerRow(el)
      }}
      className={classes}
      role="treeitem"
      aria-labelledby={p.renaming ? undefined : nameId}
      aria-label={p.renaming ? name : undefined}
      aria-level={p.depth + 1}
      aria-setsize={p.siblings.size}
      aria-posinset={p.siblings.index + 1}
      aria-expanded={p.hasChildren ? p.expanded : undefined}
      aria-selected={p.selected}
      tabIndex={p.tabbable ? 0 : -1}
      draggable={!p.renaming}
      style={{ paddingLeft: 8 + p.depth * 16 }}
      onFocus={(e) => {
        if (e.target === e.currentTarget) p.onFocusRow()
      }}
      onClick={(e) => {
        if (p.renaming) return
        if (e.target instanceof Element && e.target.closest('button, input') !== null) return
        p.onOpen(e.ctrlKey || e.metaKey)
      }}
      onContextMenu={(e) => {
        e.preventDefault()
        setMenu({ x: e.clientX, y: e.clientY, align: 'start', doc: e.currentTarget.ownerDocument })
      }}
      onDragStart={p.onDragStart}
      onDragOver={p.onDragOver}
      onDrop={p.onDrop}
      onDragEnd={p.onDragEnd}
    >
      <button
        type="button"
        className="trow__arrow"
        tabIndex={-1}
        aria-label={p.expanded ? t('wiki.tree.collapse') : t('wiki.tree.expand')}
        hidden={!p.hasChildren}
        onClick={p.onToggle}
      >
        <span aria-hidden="true">{p.expanded ? '▾' : '▸'}</span>
      </button>
      <span className="trow__icon" aria-hidden="true">
        {node.icon !== '' ? node.icon : isDatabase ? '▦' : '◦'}
      </span>
      {p.renaming ? (
        <RenameField
          label={t('wiki.tree.renameLabel')}
          initial={node.title}
          onCommit={p.onRename}
          onCancel={p.onCancelRename}
        />
      ) : (
        <span
          id={nameId}
          className={node.title === '' ? 'trow__name trow__name--empty' : 'trow__name'}
        >
          {name}
        </span>
      )}
      {isDatabase && !p.renaming && (
        <span
          className="trow__count"
          aria-label={t('wiki.tree.records', { count: node.recordCount })}
        >
          {node.recordCount}
        </span>
      )}
      {!p.renaming && (
        <span className="trow__actions">
          {!isDatabase && (
            <button
              type="button"
              className="trow__button"
              tabIndex={-1}
              aria-label={t('wiki.tree.addChild')}
              onClick={p.onAddChild}
            >
              <span aria-hidden="true">+</span>
            </button>
          )}
          <button
            ref={menuButton}
            type="button"
            className="trow__button"
            tabIndex={-1}
            aria-label={t('wiki.tree.rowMenu', { name })}
            aria-haspopup="menu"
            aria-expanded={menu !== null}
            onClick={() => {
              if (menuButton.current !== null) openMenuAt(menuButton.current)
            }}
          >
            <span aria-hidden="true">⋯</span>
          </button>
        </span>
      )}
      {menu !== null && (
        <PopupMenu
          doc={menu.doc}
          at={menu}
          label={t('wiki.tree.rowMenu', { name })}
          items={items}
          onClose={(restoreFocus) => {
            setMenu(null)
            if (restoreFocus) {
              const row = menuButton.current?.closest<HTMLElement>('[role="treeitem"]')
              row?.focus()
            }
          }}
        />
      )}
    </div>
  )
}

interface RenameFieldProps {
  label: string
  initial: string
  onCommit: (title: string) => void
  onCancel: () => void
}

/** RenameField is the name of a row turned into an input: Enter and leaving keep the name, Escape drops it. */
function RenameField({ label, initial, onCommit, onCancel }: RenameFieldProps) {
  const [value, setValue] = useState(initial)
  const input = useRef<HTMLInputElement>(null)
  // Enter and Escape end the edit first; the blur that follows the removal of the field must not end it again.
  const done = useRef(false)

  useEffect(() => {
    input.current?.focus()
    input.current?.select()
  }, [])

  function finish(commit: boolean) {
    if (done.current) return
    done.current = true
    if (commit && value !== initial) onCommit(value)
    else onCancel()
  }

  return (
    <input
      ref={input}
      className="trow__input"
      aria-label={label}
      value={value}
      maxLength={maxTitleLength}
      draggable={false}
      onChange={(e) => {
        setValue(e.target.value)
      }}
      onKeyDown={(e) => {
        e.stopPropagation()
        if (e.key === 'Enter') {
          e.preventDefault()
          finish(true)
        } else if (e.key === 'Escape') {
          e.preventDefault()
          finish(false)
        }
      }}
      onBlur={() => {
        finish(true)
      }}
    />
  )
}

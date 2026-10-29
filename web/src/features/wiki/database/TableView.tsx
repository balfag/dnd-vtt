import { memo, useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { DbRecord, Property } from '../../../shared/api/wiki'
import { PopupMenu } from '../../workspace/PanelTab'
import { InlineInput } from './values/InlineInput'
import { ValueCell } from './values/ValueCell'

const DEFAULT_WIDTH = 180
const TITLE_WIDTH = 280
const MIN_WIDTH = 40
const MAX_WIDTH = 2000
const MAX_TITLE = 200

interface Editing {
  id: string
  key: string
}

interface TableViewProps {
  columns: Property[]
  records: DbRecord[]
  widths: Record<string, number>
  /** onWidth is called once a column has been dragged to its new width. */
  onWidth: (key: string, px: number) => void
  /** errors are the texts of refused values, by record id and property id. */
  errors: Readonly<Record<string, Readonly<Record<string, string>> | undefined>>
  onClearError: (recordId: string, propId: string) => void
  onChange: (record: DbRecord, prop: Property, next: unknown) => void
  onTitle: (record: DbRecord, title: string) => void
  onOpen: (id: string) => void
  onDelete: (id: string) => void
  /** newRecord is a record just made: its title is put into editing as soon as its row is there. */
  newRecord: string | null
  onNewRecordSeen: () => void
}

function clampWidth(px: number): number {
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(px)))
}

/** TableView is the grid of a database: a row per record, a column per visible property. */
export function TableView(p: TableViewProps) {
  const { t } = useTranslation()
  const [live, setLive] = useState<Record<string, number>>({})
  const [focus, setFocus] = useState<Editing | null>(null)
  const [editing, setEditing] = useState<Editing | null>(null)
  const cells = useRef(new Map<string, HTMLElement>())
  const keys = ['title', ...p.columns.map((c) => c.id)]

  const width = (key: string) =>
    live[key] ?? p.widths[key] ?? (key === 'title' ? TITLE_WIDTH : DEFAULT_WIDTH)
  const template = keys.map((k) => `${String(width(k))}px`).join(' ')

  // force false is for the end of an edit: if the user has clicked elsewhere, the focus stays there.
  const focusCell = useCallback((id: string, key: string, force = true) => {
    setFocus({ id, key })
    // The cell may be a new one: let React draw it first.
    setTimeout(() => {
      const el = cells.current.get(`${id}:${key}`)
      if (el === undefined) return
      const active = el.ownerDocument.activeElement
      if (force || active === null || active === el.ownerDocument.body || el.contains(active))
        el.focus()
    }, 0)
  }, [])

  // A record just made has its title in editing, as soon as its row is there.
  const { newRecord, onNewRecordSeen } = p
  const pending: Editing | null =
    newRecord !== null && p.records.some((r) => r.id === newRecord)
      ? { id: newRecord, key: 'title' }
      : null
  const activeEditing = editing ?? pending

  const registerCell = useCallback((id: string, key: string, el: HTMLElement | null) => {
    if (el) cells.current.set(`${id}:${key}`, el)
    else cells.current.delete(`${id}:${key}`)
  }, [])

  function onKeyDown(e: React.KeyboardEvent) {
    const el = e.target
    if (!(el instanceof HTMLElement) || el.getAttribute('role') !== 'gridcell') return
    const id = el.dataset.record
    const key = el.dataset.col
    if (id === undefined || key === undefined) return
    const row = p.records.findIndex((r) => r.id === id)
    const col = keys.indexOf(key)
    const go = (r: number, c: number) => {
      const rec = p.records[r]
      const k = keys[c]
      if (rec !== undefined && k !== undefined) focusCell(rec.id, k)
    }
    switch (e.key) {
      case 'ArrowDown':
        go(row + 1, col)
        break
      case 'ArrowUp':
        go(row - 1, col)
        break
      case 'ArrowRight':
        go(row, col + 1)
        break
      case 'ArrowLeft':
        go(row, col - 1)
        break
      case 'Home':
        go(row, 0)
        break
      case 'End':
        go(row, keys.length - 1)
        break
      case 'F2':
        setEditing({ id, key })
        break
      case 'Enter': {
        const rec = p.records[row]
        const prop = p.columns.find((c) => c.id === key)
        if (key === 'title') {
          if (rec !== undefined) p.onOpen(rec.id)
        } else if (prop?.type === 'checkbox') {
          if (rec !== undefined) p.onChange(rec, prop, rec.props[prop.id] === true ? null : true)
        } else {
          setEditing({ id, key })
        }
        break
      }
      default:
        return
    }
    e.preventDefault()
  }

  const endEdit = useCallback(
    (id: string, key: string) => {
      setEditing(null)
      if (id === newRecord) onNewRecordSeen()
      focusCell(id, key, false)
    },
    [focusCell, newRecord, onNewRecordSeen],
  )

  function startResize(key: string, e: React.PointerEvent<HTMLElement>) {
    e.preventDefault()
    const target = e.currentTarget
    const startX = e.clientX
    const startW = width(key)
    target.setPointerCapture(e.pointerId)
    let last = startW
    const move = (ev: PointerEvent) => {
      last = clampWidth(startW + ev.clientX - startX)
      setLive((now) => ({ ...now, [key]: last }))
    }
    const up = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', up)
      target.removeEventListener('pointercancel', up)
      if (last !== startW) p.onWidth(key, last)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', up)
    target.addEventListener('pointercancel', up)
  }

  function resizeByKey(key: string, e: React.KeyboardEvent) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const next = clampWidth(width(key) + (e.key === 'ArrowRight' ? 10 : -10))
    setLive((now) => ({ ...now, [key]: next }))
    p.onWidth(key, next)
  }

  const wanted = pending ?? focus
  const tabStop = wanted !== null && p.records.some((r) => r.id === wanted.id) ? wanted : null
  const firstId = p.records[0]?.id

  return (
    <div className="dbt__scroll">
      <div
        className="dbt"
        role="grid"
        aria-label={t('wiki.db.table.label')}
        aria-rowcount={p.records.length + 1}
        aria-colcount={keys.length}
        style={{ '--cols': template } as React.CSSProperties}
        onKeyDown={onKeyDown}
      >
        <div className="dbt__row dbt__row--head" role="row">
          {keys.map((key, i) => {
            const prop = p.columns.find((c) => c.id === key)
            return (
              <div key={key} className="dbt__head" role="columnheader" aria-colindex={i + 1}>
                <span className="dbt__headname">
                  {prop === undefined ? t('wiki.db.field.title') : prop.name}
                </span>
                <span
                  className="dbt__resize"
                  role="separator"
                  aria-orientation="vertical"
                  aria-label={t('wiki.db.table.resize', {
                    name: prop === undefined ? t('wiki.db.field.title') : prop.name,
                  })}
                  aria-valuenow={width(key)}
                  aria-valuemin={MIN_WIDTH}
                  aria-valuemax={MAX_WIDTH}
                  tabIndex={0}
                  onPointerDown={(e) => {
                    startResize(key, e)
                  }}
                  onKeyDown={(e) => {
                    resizeByKey(key, e)
                  }}
                />
              </div>
            )
          })}
        </div>
        {p.records.map((r, i) => (
          <Row
            key={r.id}
            record={r}
            index={i}
            columns={p.columns}
            errors={p.errors[r.id]}
            editingKey={activeEditing?.id === r.id ? activeEditing.key : null}
            focusKey={
              tabStop?.id === r.id
                ? tabStop.key
                : tabStop === null && r.id === firstId
                  ? 'title'
                  : null
            }
            registerCell={registerCell}
            onFocusCell={setFocus}
            onEdit={setEditing}
            onEnd={endEdit}
            onClearError={p.onClearError}
            onChange={p.onChange}
            onTitle={p.onTitle}
            onOpen={p.onOpen}
            onDelete={p.onDelete}
          />
        ))}
      </div>
    </div>
  )
}

interface RowProps {
  record: DbRecord
  index: number
  columns: Property[]
  errors: Readonly<Record<string, string>> | undefined
  editingKey: string | null
  /** focusKey is the column whose cell is the one tab stop of the grid, when it is in this row. */
  focusKey: string | null
  registerCell: (id: string, key: string, el: HTMLElement | null) => void
  onFocusCell: (cell: Editing) => void
  onEdit: (cell: Editing) => void
  onEnd: (id: string, key: string) => void
  onClearError: (recordId: string, propId: string) => void
  onChange: (record: DbRecord, prop: Property, next: unknown) => void
  onTitle: (record: DbRecord, title: string) => void
  onOpen: (id: string) => void
  onDelete: (id: string) => void
}

/** Row is one record. It is memoized: typing in a cell of another row does not draw it again. */
const Row = memo(function Row(r: RowProps) {
  const { t } = useTranslation()
  const { record } = r
  const [menu, setMenu] = useState<{ x: number; y: number; align: 'end'; doc: Document } | null>(
    null,
  )
  const menuButton = useRef<HTMLButtonElement>(null)
  const name = record.title === '' ? t('wiki.untitled') : record.title

  const cellProps = (key: string, col: number) => ({
    ref: (el: HTMLDivElement | null) => {
      r.registerCell(record.id, key, el)
    },
    role: 'gridcell' as const,
    'aria-colindex': col + 1,
    'data-record': record.id,
    'data-col': key,
    tabIndex: r.focusKey === key ? 0 : -1,
    onFocus: (e: React.FocusEvent) => {
      if (e.target === e.currentTarget) r.onFocusCell({ id: record.id, key })
    },
  })

  return (
    <div className="dbt__row" role="row" aria-rowindex={r.index + 2}>
      <div
        {...cellProps('title', 0)}
        className={r.editingKey === 'title' ? 'dbt__cell dbt__cell--editing' : 'dbt__cell'}
      >
        {r.editingKey === 'title' ? (
          <InlineInput
            type="text"
            initial={record.title}
            label={t('wiki.db.table.titleOf')}
            maxLength={MAX_TITLE}
            onCommit={(text) => {
              if (text !== record.title) r.onTitle(record, text)
              r.onEnd(record.id, 'title')
            }}
            onCancel={() => {
              r.onEnd(record.id, 'title')
            }}
          />
        ) : (
          <>
            <button
              type="button"
              className="dbt__title"
              tabIndex={-1}
              onClick={() => {
                r.onOpen(record.id)
              }}
            >
              {record.icon !== '' && <span aria-hidden="true">{record.icon} </span>}
              <span className={record.title === '' ? 'dbt__untitled' : undefined}>{name}</span>
            </button>
            <button
              ref={menuButton}
              type="button"
              className="dbt__menu"
              tabIndex={-1}
              aria-label={t('wiki.db.table.rowMenu', { name })}
              aria-haspopup="menu"
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
          </>
        )}
      </div>
      {r.columns.map((prop, i) => {
        const editing = r.editingKey === prop.id
        return (
          <div
            key={prop.id}
            {...cellProps(prop.id, i + 1)}
            className={editing ? 'dbt__cell dbt__cell--editing' : 'dbt__cell'}
          >
            <ValueCell
              prop={prop}
              value={record.props[prop.id]}
              editing={editing}
              error={r.errors?.[prop.id]}
              onClear={() => {
                r.onClearError(record.id, prop.id)
              }}
              onEdit={() => {
                r.onFocusCell({ id: record.id, key: prop.id })
                r.onEdit({ id: record.id, key: prop.id })
              }}
              onEnd={() => {
                r.onEnd(record.id, prop.id)
              }}
              onChange={(next) => {
                r.onChange(record, prop, next)
              }}
            />
          </div>
        )
      })}
      {menu !== null && (
        <PopupMenu
          doc={menu.doc}
          at={menu}
          label={t('wiki.db.table.rowMenu', { name })}
          items={[
            {
              key: 'open',
              label: t('wiki.db.table.open'),
              onSelect: () => {
                r.onOpen(record.id)
              },
            },
            {
              key: 'delete',
              label: t('wiki.db.table.delete'),
              onSelect: () => {
                r.onDelete(record.id)
              },
            },
          ]}
          onClose={(restoreFocus) => {
            setMenu(null)
            if (restoreFocus) menuButton.current?.focus()
          }}
        />
      )}
    </div>
  )
})

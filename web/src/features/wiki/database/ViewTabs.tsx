import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { View, ViewKind } from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'
import { AskDelete } from './AskDelete'
import { PopupMenu } from '../../workspace/PanelTab'
import { useRestoreFocus } from './useRestoreFocus'

const MAX_VIEWS = 10
const MAX_NAME = 100

interface ViewTabsProps {
  views: View[]
  selected: string
  onSelect: (id: string) => void
  onAdd: (name: string, kind: ViewKind) => void
  onRename: (view: View, name: string) => void
  onDelete: (view: View) => void
}

type Ask = { kind: 'new' } | { kind: 'rename'; view: View } | { kind: 'delete'; view: View }

/** ViewTabs is the row of views over a database: the tabs, "+ View", and the menu "⋯" of the current one. */
export function ViewTabs({ views, selected, onSelect, onAdd, onRename, onDelete }: ViewTabsProps) {
  const { t } = useTranslation()
  const [ask, setAsk] = useState<Ask | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; align: 'end'; doc: Document } | null>(
    null,
  )
  const menuButton = useRef<HTMLButtonElement>(null)
  const refs = useRef(new Map<string, HTMLButtonElement>())
  const current = views.find((v) => v.id === selected)

  function move(from: number, step: number) {
    const next = views[(from + step + views.length) % views.length]
    if (next === undefined) return
    onSelect(next.id)
    refs.current.get(next.id)?.focus()
  }

  return (
    <div className="dbtabs">
      <div className="dbtabs__list" role="tablist" aria-label={t('wiki.db.views')}>
        {views.map((v, i) => (
          <button
            key={v.id}
            ref={(el) => {
              if (el) refs.current.set(v.id, el)
              else refs.current.delete(v.id)
            }}
            type="button"
            role="tab"
            className="dbtabs__tab"
            aria-selected={v.id === selected}
            tabIndex={v.id === selected ? 0 : -1}
            onClick={() => {
              onSelect(v.id)
            }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowRight') {
                e.preventDefault()
                move(i, 1)
              } else if (e.key === 'ArrowLeft') {
                e.preventDefault()
                move(i, -1)
              }
            }}
          >
            {v.name}
          </button>
        ))}
      </div>
      {current !== undefined && (
        <button
          ref={menuButton}
          type="button"
          className="dbtabs__button"
          aria-label={t('wiki.db.view.menu', { name: current.name })}
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
      <button
        type="button"
        className="dbtabs__add"
        disabled={views.length >= MAX_VIEWS}
        onClick={() => {
          setAsk({ kind: 'new' })
        }}
      >
        {t('wiki.db.view.add')}
      </button>

      {menu !== null && current !== undefined && (
        <PopupMenu
          doc={menu.doc}
          at={menu}
          label={t('wiki.db.view.menu', { name: current.name })}
          items={[
            {
              key: 'rename',
              label: t('wiki.db.view.rename'),
              onSelect: () => {
                setAsk({ kind: 'rename', view: current })
              },
            },
            ...(views.length > 1
              ? [
                  {
                    key: 'delete',
                    label: t('wiki.db.view.delete'),
                    onSelect: () => {
                      setAsk({ kind: 'delete', view: current })
                    },
                  },
                ]
              : []),
          ]}
          onClose={(restoreFocus) => {
            setMenu(null)
            if (restoreFocus) menuButton.current?.focus()
          }}
        />
      )}

      {ask?.kind === 'delete' && (
        <AskDelete
          title={t('wiki.db.view.deleteTitle', { name: ask.view.name })}
          text={t('wiki.db.view.deleteConfirm')}
          onConfirm={() => {
            onDelete(ask.view)
          }}
          onClose={() => {
            setAsk(null)
          }}
        />
      )}
      {(ask?.kind === 'new' || ask?.kind === 'rename') && (
        <ViewDialog
          ask={ask}
          onClose={() => {
            setAsk(null)
          }}
          onSubmit={(name, kind) => {
            if (ask.kind === 'new') onAdd(name, kind)
            else onRename(ask.view, name)
            setAsk(null)
          }}
        />
      )}
    </div>
  )
}

interface ViewDialogProps {
  ask: { kind: 'new' } | { kind: 'rename'; view: View }
  onClose: () => void
  onSubmit: (name: string, kind: ViewKind) => void
}

/** ViewDialog asks the name of a view and, for a new one, whether it is a table or cards. */
function ViewDialog({ ask, onClose, onSubmit }: ViewDialogProps) {
  const { t } = useTranslation()
  useRestoreFocus()
  const [name, setName] = useState(ask.kind === 'rename' ? ask.view.name : '')
  const [kind, setKind] = useState<ViewKind>('table')
  const nameId = useId()
  const kindId = useId()
  const trimmed = name.trim()
  const valid = trimmed !== '' && trimmed.length <= MAX_NAME

  return (
    <Dialog
      title={ask.kind === 'new' ? t('wiki.db.view.newTitle') : t('wiki.db.view.rename')}
      open
      closeLabel={t('ui.dialog.cancel')}
      onClose={onClose}
      actions={
        <Button
          fullWidth={false}
          disabled={!valid}
          onClick={() => {
            onSubmit(trimmed, kind)
          }}
        >
          {ask.kind === 'new' ? t('wiki.db.view.create') : t('wiki.db.view.save')}
        </Button>
      }
    >
      <form
        className="dbform"
        onSubmit={(e) => {
          e.preventDefault()
          if (valid) onSubmit(trimmed, kind)
        }}
      >
        <label className="dbform__label" htmlFor={nameId}>
          {t('wiki.db.view.name')}
        </label>
        <input
          id={nameId}
          className="field__input"
          value={name}
          maxLength={MAX_NAME}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        {ask.kind === 'new' && (
          <>
            <label className="dbform__label" htmlFor={kindId}>
              {t('wiki.db.view.kind')}
            </label>
            <select
              id={kindId}
              className="field__input"
              value={kind}
              onChange={(e) => {
                setKind(e.target.value === 'cards' ? 'cards' : 'table')
              }}
            >
              <option value="table">{t('wiki.db.view.table')}</option>
              <option value="cards">{t('wiki.db.view.cards')}</option>
            </select>
          </>
        )}
      </form>
    </Dialog>
  )
}

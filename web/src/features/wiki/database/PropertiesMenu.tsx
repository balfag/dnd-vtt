import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  optionColors,
  type Property,
  type PropertyOptions,
  type SelectOption,
} from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'
import { colorValue, nextColor, optionsOf } from './model'

const MAX_NAME = 100
const MAX_OPTIONS = 100

interface PropertiesMenuProps {
  properties: Property[]
  /** visible lists the properties shown in the current view, in the order of its columns. */
  visible: string[]
  onVisible: (ids: string[]) => void
  onMove: (prop: Property, delta: -1 | 1) => void
  onRename: (prop: Property, name: string) => Promise<boolean>
  onOptions: (prop: Property, options: PropertyOptions, cleared: boolean) => void
  onDelete: (prop: Property) => void
  onAdd: () => void
}

type Confirm =
  { kind: 'property'; prop: Property } | { kind: 'option'; prop: Property; index: number }

/** PropertiesMenu lists the properties of a database: shown in this view or not, order, name, variants, delete. */
export function PropertiesMenu(p: PropertiesMenuProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<Confirm | null>(null)

  function toggle(prop: Property, shown: boolean) {
    if (shown) {
      // Keep the columns in the order of the properties.
      const ids = new Set([...p.visible, prop.id])
      p.onVisible(p.properties.map((x) => x.id).filter((id) => ids.has(id)))
    } else {
      p.onVisible(p.visible.filter((id) => id !== prop.id))
    }
  }

  function setOptionList(prop: Property, list: SelectOption[], cleared: boolean) {
    p.onOptions(prop, { ...prop.options, options: list }, cleared)
  }

  return (
    <div className="dbedit">
      {p.properties.length === 0 && <p className="dbedit__note">{t('wiki.db.property.none')}</p>}
      <ul className="dbedit__rows dbprops">
        {p.properties.map((prop, i) => {
          const shown = p.visible.includes(prop.id)
          const hasOptions = prop.type === 'select' || prop.type === 'multi_select'
          return (
            <li key={prop.id} className="dbprops__item">
              <div className="dbedit__row">
                <input
                  type="checkbox"
                  aria-label={t('wiki.db.property.shown', { name: prop.name })}
                  checked={shown}
                  onChange={(e) => {
                    toggle(prop, e.target.checked)
                  }}
                />
                <input
                  className="dbedit__input"
                  aria-label={t('wiki.db.property.rename', { name: prop.name })}
                  defaultValue={prop.name}
                  maxLength={MAX_NAME}
                  onBlur={(e) => {
                    const name = e.target.value.trim()
                    const input = e.target
                    if (name === '' || name === prop.name) input.value = prop.name
                    else
                      void p.onRename(prop, name).then((ok) => {
                        if (!ok) input.value = prop.name
                      })
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') e.currentTarget.blur()
                  }}
                />
                <span className="dbprops__type">{t(`wiki.db.type.${prop.type}`)}</span>
                <button
                  type="button"
                  className="dbedit__remove"
                  aria-label={t('wiki.db.property.up', { name: prop.name })}
                  disabled={i === 0}
                  onClick={() => {
                    p.onMove(prop, -1)
                  }}
                >
                  <span aria-hidden="true">↑</span>
                </button>
                <button
                  type="button"
                  className="dbedit__remove"
                  aria-label={t('wiki.db.property.down', { name: prop.name })}
                  disabled={i === p.properties.length - 1}
                  onClick={() => {
                    p.onMove(prop, 1)
                  }}
                >
                  <span aria-hidden="true">↓</span>
                </button>
                <button
                  type="button"
                  className="dbedit__remove"
                  aria-label={t('wiki.db.property.delete', { name: prop.name })}
                  onClick={() => {
                    setConfirm({ kind: 'property', prop })
                  }}
                >
                  <span aria-hidden="true">✕</span>
                </button>
              </div>
              {hasOptions && (
                <div className="dbprops__options">
                  <button
                    type="button"
                    className="dbedit__link"
                    aria-expanded={open === prop.id}
                    onClick={() => {
                      setOpen(open === prop.id ? null : prop.id)
                    }}
                  >
                    {t('wiki.db.option.title', { count: optionsOf(prop).length })}
                  </button>
                  {open === prop.id && (
                    <ul className="dbedit__rows">
                      {optionsOf(prop).map((o, j) => (
                        <li key={o.id ?? j} className="dbedit__row">
                          <span
                            className="dbopt__mark"
                            aria-hidden="true"
                            style={{ backgroundColor: colorValue[o.color] }}
                          />
                          <input
                            className="dbedit__input"
                            aria-label={t('wiki.db.option.name')}
                            defaultValue={o.name}
                            maxLength={MAX_NAME}
                            onBlur={(e) => {
                              const name = e.target.value.trim()
                              if (name === '' || name === o.name) {
                                e.target.value = o.name
                                return
                              }
                              setOptionList(
                                prop,
                                optionsOf(prop).map((x, k) => (k === j ? { ...x, name } : x)),
                                false,
                              )
                            }}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') e.currentTarget.blur()
                            }}
                          />
                          <select
                            className="dbedit__select"
                            aria-label={t('wiki.db.option.color', { name: o.name })}
                            value={o.color}
                            onChange={(e) => {
                              const color = optionColors.find((c) => c === e.target.value)
                              if (color === undefined) return
                              setOptionList(
                                prop,
                                optionsOf(prop).map((x, k) => (k === j ? { ...x, color } : x)),
                                false,
                              )
                            }}
                          >
                            {optionColors.map((c) => (
                              <option key={c} value={c}>
                                {t(`wiki.db.color.${c}`)}
                              </option>
                            ))}
                          </select>
                          <button
                            type="button"
                            className="dbedit__remove"
                            aria-label={t('wiki.db.option.delete', { name: o.name })}
                            onClick={() => {
                              setConfirm({ kind: 'option', prop, index: j })
                            }}
                          >
                            <span aria-hidden="true">✕</span>
                          </button>
                        </li>
                      ))}
                      <li>
                        <button
                          type="button"
                          className="dbedit__add"
                          disabled={optionsOf(prop).length >= MAX_OPTIONS}
                          onClick={() => {
                            const list = optionsOf(prop)
                            setOptionList(
                              prop,
                              [
                                ...list,
                                {
                                  name: newOptionName(t('wiki.db.option.new'), list),
                                  color: nextColor(list),
                                },
                              ],
                              false,
                            )
                          }}
                        >
                          {t('wiki.db.option.add')}
                        </button>
                      </li>
                    </ul>
                  )}
                </div>
              )}
            </li>
          )
        })}
      </ul>
      <button
        type="button"
        className="dbedit__add"
        disabled={p.properties.length >= 30}
        onClick={p.onAdd}
      >
        {t('wiki.db.property.add')}
      </button>

      {confirm !== null && (
        <Dialog
          title={
            confirm.kind === 'property'
              ? t('wiki.db.property.deleteTitle', { name: confirm.prop.name })
              : t('wiki.db.option.deleteTitle', {
                  name: optionsOf(confirm.prop)[confirm.index]?.name ?? '',
                })
          }
          open
          closeLabel={t('ui.dialog.cancel')}
          onClose={() => {
            setConfirm(null)
          }}
          actions={
            <Button
              variant="danger"
              fullWidth={false}
              onClick={() => {
                if (confirm.kind === 'property') {
                  p.onDelete(confirm.prop)
                } else {
                  setOptionList(
                    confirm.prop,
                    optionsOf(confirm.prop).filter((_, k) => k !== confirm.index),
                    true,
                  )
                }
                setConfirm(null)
              }}
            >
              {t('wiki.db.delete')}
            </Button>
          }
        >
          <p>
            {confirm.kind === 'property'
              ? t('wiki.db.property.deleteConfirm')
              : t('wiki.db.option.deleteConfirm')}
          </p>
        </Dialog>
      )}
    </div>
  )
}

/** newOptionName is a name for a new variant that no other variant has: "New", "New 2", ... */
function newOptionName(base: string, list: SelectOption[]): string {
  const taken = new Set(list.map((o) => o.name))
  if (!taken.has(base)) return base
  for (let n = 2; ; n++) {
    const name = `${base} ${String(n)}`
    if (!taken.has(name)) return name
  }
}

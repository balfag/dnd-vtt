import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listTags, type Filter, type Property, type Tag } from '../../../shared/api/wiki'
import { fieldLabel, isComplete, opLabel, opsOf, valueKindOf } from './describe'
import { fieldOps, localDate } from './filter'
import { optionsOf } from './model'

const MAX_FILTERS = 20
const MAX_TEXT = 200

/** today is the date a new date condition starts with. */
function today(): string {
  return localDate(Date.now())
}

interface FilterEditorProps {
  campaignId: string
  properties: Property[]
  filters: Filter[]
  /** onChange gets the filters that are complete; a row still being filled stays here only. */
  onChange: (filters: Filter[]) => void
}

/** keyOf is the value of the field selector of a row: "p:<id>" for a property, "f:<name>" for a built-in field. */
function keyOf(f: Filter): string {
  return f.propertyId !== undefined ? `p:${f.propertyId}` : `f:${f.field ?? 'title'}`
}

/** FilterEditor edits the conditions of a view: rows "field · operator · value", all joined by "and". */
export function FilterEditor({ campaignId, properties, filters, onChange }: FilterEditorProps) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<Filter[]>(filters)
  const [tags, setTags] = useState<Tag[]>([])

  useEffect(() => {
    let stale = false
    listTags(campaignId).then(
      (list) => {
        if (!stale) setTags(list)
      },
      () => undefined,
    )
    return () => {
      stale = true
    }
  }, [campaignId])

  function commit(next: Filter[]) {
    setRows(next)
    onChange(next.filter((f) => isComplete(f, properties)))
  }

  function blank(key: string): Filter {
    const prop = key.startsWith('p:') ? properties.find((p) => p.id === key.slice(2)) : undefined
    const base: Filter =
      prop !== undefined ? { propertyId: prop.id, op: '' } : { field: fieldOf(key), op: '' }
    const op = opsOf(prop, base.field)[0]?.[0] ?? 'empty'
    return withDefault({ ...base, op }, prop)
  }

  function withDefault(f: Filter, prop: Property | undefined): Filter {
    const out: Filter = { ...f }
    delete out.value
    switch (valueKindOf(prop, f.field, f.op)) {
      case 'option': {
        const first = optionsOf(prop ?? emptyProp)[0]?.id
        if (first !== undefined) out.value = first
        break
      }
      case 'date':
        out.value = today()
        break
      case 'tag': {
        const first = tags[0]?.id
        if (first !== undefined) out.value = first
        break
      }
      default:
        break
    }
    return out
  }

  function setRow(i: number, f: Filter) {
    commit(rows.map((r, j) => (j === i ? f : r)))
  }

  const firstKey = properties[0] !== undefined ? `p:${properties[0].id}` : 'f:title'

  return (
    <div className="dbedit">
      {rows.length === 0 && <p className="dbedit__note">{t('wiki.db.filter.none')}</p>}
      <ul className="dbedit__rows">
        {rows.map((f, i) => {
          const prop = properties.find((p) => p.id === f.propertyId)
          const kind = valueKindOf(prop, f.field, f.op)
          const name = fieldLabel(t, properties, f)
          return (
            <li key={i} className="dbedit__row">
              <select
                className="dbedit__select"
                aria-label={t('wiki.db.filter.field')}
                value={keyOf(f)}
                onChange={(e) => {
                  setRow(i, blank(e.target.value))
                }}
              >
                {properties.map((p) => (
                  <option key={p.id} value={`p:${p.id}`}>
                    {p.name}
                  </option>
                ))}
                {Object.keys(fieldOps).map((field) => (
                  <option key={field} value={`f:${field}`}>
                    {t(`wiki.db.field.${field}`)}
                  </option>
                ))}
              </select>
              <select
                className="dbedit__select"
                aria-label={t('wiki.db.filter.op', { name })}
                value={f.op}
                onChange={(e) => {
                  setRow(i, withDefault({ ...f, op: e.target.value }, prop))
                }}
              >
                {opsOf(prop, f.field).map(([op]) => (
                  <option key={op} value={op}>
                    {opLabel(t, prop, op)}
                  </option>
                ))}
              </select>
              {kind === 'text' && (
                <input
                  className="dbedit__input"
                  aria-label={t('wiki.db.filter.value', { name })}
                  maxLength={MAX_TEXT}
                  value={typeof f.value === 'string' ? f.value : ''}
                  onChange={(e) => {
                    setRow(i, { ...f, value: e.target.value })
                  }}
                />
              )}
              {kind === 'number' && (
                <input
                  className="dbedit__input"
                  type="number"
                  step="any"
                  aria-label={t('wiki.db.filter.value', { name })}
                  value={typeof f.value === 'number' ? f.value : ''}
                  onChange={(e) => {
                    const n = e.target.valueAsNumber
                    const next = { ...f }
                    if (Number.isFinite(n)) next.value = n
                    else delete next.value
                    setRow(i, next)
                  }}
                />
              )}
              {kind === 'date' && (
                <input
                  className="dbedit__input"
                  type="date"
                  aria-label={t('wiki.db.filter.value', { name })}
                  value={typeof f.value === 'string' ? f.value : ''}
                  onChange={(e) => {
                    const next = { ...f }
                    if (e.target.value === '') delete next.value
                    else next.value = e.target.value
                    setRow(i, next)
                  }}
                />
              )}
              {kind === 'option' && (
                <select
                  className="dbedit__select"
                  aria-label={t('wiki.db.filter.value', { name })}
                  value={typeof f.value === 'string' ? f.value : ''}
                  onChange={(e) => {
                    setRow(i, { ...f, value: e.target.value })
                  }}
                >
                  {optionsOf(prop ?? emptyProp).map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.name}
                    </option>
                  ))}
                </select>
              )}
              {kind === 'tag' && (
                <select
                  className="dbedit__select"
                  aria-label={t('wiki.db.filter.value', { name })}
                  value={typeof f.value === 'string' ? f.value : ''}
                  onChange={(e) => {
                    setRow(i, { ...f, value: e.target.value })
                  }}
                >
                  {tags.map((tag) => (
                    <option key={tag.id} value={tag.id}>
                      {tag.name}
                    </option>
                  ))}
                </select>
              )}
              <button
                type="button"
                className="dbedit__remove"
                aria-label={t('wiki.db.filter.remove')}
                onClick={() => {
                  commit(rows.filter((_, j) => j !== i))
                }}
              >
                <span aria-hidden="true">✕</span>
              </button>
            </li>
          )
        })}
      </ul>
      <button
        type="button"
        className="dbedit__add"
        disabled={rows.length >= MAX_FILTERS}
        onClick={() => {
          commit([...rows, blank(firstKey)])
        }}
      >
        {t('wiki.db.filter.add')}
      </button>
    </div>
  )
}

const emptyProp: Property = { id: '', name: '', type: 'select', position: 0, options: {} }

function fieldOf(key: string): Filter['field'] {
  const f = key.slice(2)
  return f === 'createdAt' || f === 'updatedAt' || f === 'tags' ? f : 'title'
}

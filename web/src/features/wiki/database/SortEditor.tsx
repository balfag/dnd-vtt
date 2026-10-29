import { useTranslation } from 'react-i18next'
import type { Property, SortKey } from '../../../shared/api/wiki'
import { fieldLabel } from './describe'
import { sortFields } from './filter'

const MAX_SORTS = 3

interface SortEditorProps {
  properties: Property[]
  sorts: SortKey[]
  onChange: (sorts: SortKey[]) => void
}

function keyOf(s: SortKey): string {
  return s.propertyId !== undefined ? `p:${s.propertyId}` : `f:${s.field ?? 'title'}`
}

function fromKey(key: string, dir: SortKey['dir']): SortKey {
  if (key.startsWith('p:')) return { propertyId: key.slice(2), dir }
  const f = key.slice(2)
  return { field: f === 'createdAt' || f === 'updatedAt' ? f : 'title', dir }
}

/** SortEditor edits up to three sort keys of a view; the first one counts most. */
export function SortEditor({ properties, sorts, onChange }: SortEditorProps) {
  const { t } = useTranslation()

  function nextKey(): string {
    const used = new Set(sorts.map(keyOf))
    const all = [...sortFields.map((f) => `f:${f}`), ...properties.map((p) => `p:${p.id}`)]
    return all.find((k) => !used.has(k)) ?? 'f:title'
  }

  return (
    <div className="dbedit">
      {sorts.length === 0 && <p className="dbedit__note">{t('wiki.db.sort.none')}</p>}
      <ul className="dbedit__rows">
        {sorts.map((s, i) => {
          const name = fieldLabel(t, properties, s)
          return (
            <li key={i} className="dbedit__row">
              <select
                className="dbedit__select"
                aria-label={t('wiki.db.sort.key', { n: i + 1 })}
                value={keyOf(s)}
                onChange={(e) => {
                  onChange(sorts.map((x, j) => (j === i ? fromKey(e.target.value, x.dir) : x)))
                }}
              >
                {sortFields.map((f) => (
                  <option key={f} value={`f:${f}`}>
                    {t(`wiki.db.field.${f}`)}
                  </option>
                ))}
                {properties.map((p) => (
                  <option key={p.id} value={`p:${p.id}`}>
                    {p.name}
                  </option>
                ))}
              </select>
              <select
                className="dbedit__select"
                aria-label={t('wiki.db.sort.dir', { name })}
                value={s.dir}
                onChange={(e) => {
                  const dir = e.target.value === 'desc' ? 'desc' : 'asc'
                  onChange(sorts.map((x, j) => (j === i ? { ...x, dir } : x)))
                }}
              >
                <option value="asc">{t('wiki.db.sort.asc')}</option>
                <option value="desc">{t('wiki.db.sort.desc')}</option>
              </select>
              <button
                type="button"
                className="dbedit__remove"
                aria-label={t('wiki.db.sort.remove')}
                onClick={() => {
                  onChange(sorts.filter((_, j) => j !== i))
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
        disabled={sorts.length >= MAX_SORTS}
        onClick={() => {
          onChange([...sorts, fromKey(nextKey(), 'asc')])
        }}
      >
        {t('wiki.db.sort.add')}
      </button>
    </div>
  )
}

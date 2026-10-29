import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { DbRecord, Property } from '../../../../shared/api/wiki'
import { idsOf, listLimit } from '../model'
import { useValueEnv } from './env'
import { PickerPopup } from './PickerPopup'

function recordText(r: DbRecord, untitled: string): string {
  return [r.icon, r.title === '' ? untitled : r.title].filter((s) => s !== '').join(' ')
}

/** RelationView lists the records a relation points at; a record that is deleted or in the trash is not shown. */
export function RelationView({ prop, value }: { prop: Property; value: unknown }) {
  const { t } = useTranslation()
  const env = useValueEnv()
  const target = env.relations.get(prop.options.targetDatabaseId ?? '')
  const records = idsOf(value).flatMap((id) => target?.get(id) ?? [])
  if (records.length === 0) return null
  return (
    <span className="dbv__list">
      {records.map((r) => (
        <button
          key={r.id}
          type="button"
          className="dbv__link"
          onClick={(e) => {
            e.stopPropagation()
            env.openRecord(r.id)
          }}
        >
          {recordText(r, t('wiki.untitled'))}
        </button>
      ))}
    </span>
  )
}

interface RelationEditorProps {
  prop: Property
  value: unknown
  anchor: HTMLElement
  label: string
  onChange: (next: unknown) => void
  onEnd: () => void
}

/** RelationEditor searches the records of the target database by title. */
export function RelationEditor({
  prop,
  value,
  anchor,
  label,
  onChange,
  onEnd,
}: RelationEditorProps) {
  const { t } = useTranslation()
  const env = useValueEnv()
  const [query, setQuery] = useState('')
  const target = env.relations.get(prop.options.targetDatabaseId ?? '')
  const selected = idsOf(value).filter((id) => target?.has(id) === true)
  const q = query.trim().toLocaleLowerCase()
  const items = [...(target?.values() ?? [])]
    .filter((r) => r.title.toLocaleLowerCase().includes(q))
    .slice(0, 100)
    .map((r) => ({
      id: r.id,
      text: r.title,
      selected: selected.includes(r.id),
      label: <span className="dbpick__text">{recordText(r, t('wiki.untitled'))}</span>,
    }))

  function pick(id: string) {
    const next = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
    onChange(next.length === 0 ? null : next)
  }

  return (
    <PickerPopup
      anchor={anchor}
      label={label}
      items={items}
      loading={target === undefined}
      query={query}
      onQuery={setQuery}
      onPick={pick}
      full={selected.length >= listLimit(prop)}
      onClose={onEnd}
    />
  )
}

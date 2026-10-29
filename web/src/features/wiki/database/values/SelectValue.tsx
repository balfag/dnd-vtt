import { useState } from 'react'
import { errorKey } from '../../../../shared/api/errors'
import type { Property } from '../../../../shared/api/wiki'
import { idsOf, listLimit, optionById, optionsOf } from '../model'
import { useValueEnv } from './env'
import { OptionMark } from './OptionMark'
import { PickerPopup } from './PickerPopup'

const MAX_OPTIONS = 100
const MAX_NAME = 100

/** SelectView shows the chosen variants as a colored square and a word each. */
export function SelectView({ prop, value }: { prop: Property; value: unknown }) {
  const ids = prop.type === 'select' ? (typeof value === 'string' ? [value] : []) : idsOf(value)
  const chosen = ids.flatMap((id) => optionById(prop, id) ?? [])
  if (chosen.length === 0) return null
  return (
    <span className="dbv__list">
      {chosen.map((o) => (
        <OptionMark key={o.id} option={o} />
      ))}
    </span>
  )
}

interface SelectEditorProps {
  prop: Property
  value: unknown
  anchor: HTMLElement
  label: string
  onChange: (next: unknown) => void
  onError: (key: string) => void
  onEnd: () => void
}

/** SelectEditor chooses variants of a select or a multi_select; "Create «…»" adds a variant at once. */
export function SelectEditor({
  prop,
  value,
  anchor,
  label,
  onChange,
  onError,
  onEnd,
}: SelectEditorProps) {
  const env = useValueEnv()
  const [query, setQuery] = useState('')
  const multi = prop.type === 'multi_select'
  const selected = multi ? idsOf(value) : typeof value === 'string' ? [value] : []
  const all = optionsOf(prop)
  const q = query.trim().toLocaleLowerCase()
  const items = all
    .filter((o) => o.id !== undefined && o.name.toLocaleLowerCase().includes(q))
    .map((o) => ({
      id: o.id ?? '',
      text: o.name,
      selected: selected.includes(o.id ?? ''),
      label: <OptionMark option={o} />,
    }))

  function apply(ids: string[]) {
    if (multi) onChange(ids.length === 0 ? null : ids)
    else onChange(ids[0] ?? null)
  }

  function pick(id: string) {
    if (multi) {
      apply(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id])
    } else {
      apply(selected.includes(id) ? [] : [id])
      onEnd()
    }
  }

  function create(name: string) {
    env.createOption(prop, name).then(
      (id) => {
        apply(multi ? [...selected, id] : [id])
        if (!multi) onEnd()
        else setQuery('')
      },
      (err: unknown) => {
        onError(errorKey(err))
        onEnd()
      },
    )
  }

  return (
    <PickerPopup
      anchor={anchor}
      label={label}
      items={items}
      query={query}
      onQuery={setQuery}
      onPick={pick}
      onCreate={all.length < MAX_OPTIONS && query.trim().length <= MAX_NAME ? create : undefined}
      full={multi && selected.length >= listLimit(prop)}
      onClose={onEnd}
    />
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Property } from '../../../../shared/api/wiki'
import { isHttpUrl, limits, parseNumber, sameValue } from '../model'
import { GameLinkEditor, GameLinkView } from './GameLinkValue'
import { InlineInput } from './InlineInput'
import { RelationEditor, RelationView } from './RelationValue'
import { SelectEditor, SelectView } from './SelectValue'

type InlineType = 'text' | 'number' | 'date' | 'url'

/** fieldType is the kind of field that edits a type in place, or null when the type has a list or a checkbox. */
function fieldType(type: Property['type']): InlineType | null {
  return type === 'text' || type === 'number' || type === 'date' || type === 'url' ? type : null
}

/** textOf is the text a field starts with. */
function textOf(v: unknown): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number') return String(v)
  return ''
}

function formatDate(v: string, lang: string): string {
  const [y, m, d] = v.split('-').map(Number)
  if (y === undefined || m === undefined || d === undefined) return v
  return new Date(y, m - 1, d).toLocaleDateString(lang, { dateStyle: 'medium' })
}

export interface ValueCellProps {
  prop: Property
  value: unknown
  editing: boolean
  /** onEdit asks to start editing (a click on the value). */
  onEdit: () => void
  /** onEnd tells that the editing is over. */
  onEnd: () => void
  /** onChange stores the new value at once; null removes it. */
  onChange: (next: unknown) => void
  /** error is the text of what the server refused, shown under the value. */
  error?: string
  /** onClear forgets the error of the value. */
  onClear?: () => void
}

/**
 * ValueCell shows the value of a property by its type and, while editing, the editor of the type.
 * It is the same in a table cell and in the header of a record.
 */
export function ValueCell(p: ValueCellProps) {
  const { t, i18n } = useTranslation()
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [refused, setRefused] = useState<string | null>(null)
  const { prop, value } = p
  const label = prop.name

  function change(next: unknown) {
    if (!sameValue(next, value)) {
      setRefused(null)
      p.onClear?.()
      p.onChange(next)
    }
  }

  function field(type: InlineType) {
    const integer = prop.options.format === 'integer'
    return (
      <InlineInput
        type={type}
        initial={textOf(value)}
        label={label}
        maxLength={type === 'text' || type === 'url' ? limits.text : undefined}
        step={type === 'number' ? (integer ? '1' : 'any') : undefined}
        validate={(text) => {
          if (type === 'number') {
            return parseNumber(text, integer) === undefined ? 'wiki.db.value.notNumber' : null
          }
          if (type === 'url' && text.trim() !== '' && !isHttpUrl(text.trim())) {
            return 'wiki.db.value.notUrl'
          }
          return null
        }}
        onCommit={(text) => {
          const s = text.trim()
          if (type === 'number') change(parseNumber(s, integer) ?? null)
          else change(s === '' ? null : s)
          p.onEnd()
        }}
        onCancel={(why) => {
          if (why !== undefined) setRefused(why)
          p.onEnd()
        }}
      />
    )
  }

  function display() {
    switch (prop.type) {
      case 'text':
        return typeof value === 'string' ? <span className="dbv__text">{value}</span> : null
      case 'number':
        return typeof value === 'number' ? (
          <span className="dbv__text dbv__text--number">
            {value.toLocaleString(i18n.language, { maximumFractionDigits: 12 })}
          </span>
        ) : null
      case 'date':
        return typeof value === 'string' ? (
          <span className="dbv__text">{formatDate(value, i18n.language)}</span>
        ) : null
      case 'url':
        return typeof value === 'string' && value !== '' ? (
          isHttpUrl(value) ? (
            <a
              className="dbv__url"
              href={value}
              target="_blank"
              rel="noopener noreferrer"
              onClick={(e) => {
                e.stopPropagation()
              }}
            >
              {value}
            </a>
          ) : (
            <span className="dbv__text">{value}</span>
          )
        ) : null
      case 'select':
      case 'multi_select':
        return <SelectView prop={prop} value={value} />
      case 'relation':
        return <RelationView prop={prop} value={value} />
      case 'game_link':
        return <GameLinkView value={value} />
      case 'checkbox':
        return null
    }
  }

  const inlineType = fieldType(prop.type)
  const problem = refused !== null ? t(refused) : p.error

  let body: React.ReactNode
  if (prop.type === 'checkbox') {
    body = (
      <input
        type="checkbox"
        className="dbv__check"
        aria-label={label}
        checked={value === true}
        onChange={(e) => {
          change(e.target.checked ? true : null)
        }}
      />
    )
  } else if (p.editing && inlineType !== null) {
    body = field(inlineType)
  } else {
    body = display()
  }

  return (
    <div
      ref={setAnchor}
      className="dbv"
      onClick={() => {
        if (!p.editing && prop.type !== 'checkbox') {
          setRefused(null)
          p.onEdit()
        }
      }}
    >
      {body}
      {p.editing && anchor !== null && (prop.type === 'select' || prop.type === 'multi_select') && (
        <SelectEditor
          prop={prop}
          value={value}
          anchor={anchor}
          label={label}
          onChange={change}
          onError={setRefused}
          onEnd={p.onEnd}
        />
      )}
      {p.editing && anchor !== null && prop.type === 'relation' && (
        <RelationEditor
          prop={prop}
          value={value}
          anchor={anchor}
          label={label}
          onChange={change}
          onEnd={p.onEnd}
        />
      )}
      {p.editing && anchor !== null && prop.type === 'game_link' && (
        <GameLinkEditor
          value={value}
          anchor={anchor}
          label={label}
          onChange={change}
          onEnd={p.onEnd}
        />
      )}
      {problem !== undefined && !(p.editing && inlineType !== null) && (
        <p className="dbv__error" role="alert">
          {problem}
        </p>
      )}
    </div>
  )
}

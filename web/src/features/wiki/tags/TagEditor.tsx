import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listTags, type Tag } from '../../../shared/api/wiki'
import { suggestTags } from '../search/query'

const MAX_TAGS = 20
const MAX_NAME = 40

interface TagEditorProps {
  campaignId: string
  tags: Tag[]
  /** onChange gets the whole new list of names; the page saves it at once. */
  onChange: (names: string[]) => void
}

/** TagEditor is the row of tags under the title: words, "✕" on hover, and "+ tag" with suggestions. */
export function TagEditor({ campaignId, tags, onChange }: TagEditorProps) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState('')
  const [known, setKnown] = useState<string[]>([])
  const [pick, setPick] = useState(-1)
  const input = useRef<HTMLInputElement>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  const listId = useId()
  const names = tags.map((x) => x.name)
  const options = adding ? suggestTags(known, names, draft).slice(0, 8) : []
  const full = names.length >= MAX_TAGS

  useEffect(() => {
    if (!adding) return
    input.current?.focus()
    let stale = false
    listTags(campaignId).then(
      (list) => {
        if (!stale) setKnown(list.map((x) => x.name))
      },
      () => undefined,
    )
    return () => {
      stale = true
    }
  }, [adding, campaignId])

  function add(raw: string) {
    const name = raw.trim()
    if (name === '' || full) return
    setDraft('')
    setPick(-1)
    if (names.some((n) => n.toLowerCase() === name.toLowerCase())) return
    onChange([...names, name])
  }

  function close() {
    setAdding(false)
    setDraft('')
    setPick(-1)
  }

  return (
    <div className="ptags" role="group" aria-label={t('wiki.tools.tags.page')}>
      {tags.map((tag) => (
        <span key={tag.id} className="ptags__tag">
          {tag.name}
          <button
            type="button"
            className="ptags__remove"
            aria-label={t('wiki.tools.tags.remove', { name: tag.name })}
            onClick={() => {
              onChange(names.filter((n) => n !== tag.name))
            }}
          >
            <span aria-hidden="true">✕</span>
          </button>
        </span>
      ))}
      {adding ? (
        <span className="ptags__field">
          <input
            ref={input}
            className="ptags__input"
            role="combobox"
            aria-label={t('wiki.tools.tags.addLabel')}
            aria-expanded={options.length > 0}
            aria-controls={listId}
            aria-autocomplete="list"
            aria-activedescendant={pick >= 0 ? `${listId}-${String(pick)}` : undefined}
            maxLength={MAX_NAME}
            autoComplete="off"
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value)
              setPick(-1)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                add(pick >= 0 ? (options[pick] ?? draft) : draft)
              } else if (e.key === 'Backspace' && draft === '') {
                if (names.length > 0) {
                  e.preventDefault()
                  onChange(names.slice(0, -1))
                }
              } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                e.preventDefault()
                if (options.length > 0) {
                  const step = e.key === 'ArrowDown' ? 1 : -1
                  // -1 is the typed text itself; the suggestions are 0 and up.
                  const total = options.length + 1
                  setPick(((((pick + 1 + step) % total) + total) % total) - 1)
                }
              } else if (e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation()
                close()
                addButton.current?.focus()
              }
            }}
            onBlur={() => {
              // A click on a suggestion keeps the focus (see onMouseDown there), so blur means leaving.
              if (draft.trim() !== '') add(draft)
              close()
            }}
          />
          {options.length > 0 && (
            <ul
              id={listId}
              className="ptags__list"
              role="listbox"
              aria-label={t('wiki.tools.tags.suggestions')}
            >
              {options.map((name, i) => (
                <li
                  key={name}
                  id={`${listId}-${String(i)}`}
                  role="option"
                  aria-selected={i === pick}
                  className={i === pick ? 'ptags__option ptags__option--active' : 'ptags__option'}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    add(name)
                  }}
                >
                  {name}
                </li>
              ))}
            </ul>
          )}
        </span>
      ) : (
        !full && (
          <button
            ref={addButton}
            type="button"
            className="ptags__add"
            onClick={() => {
              setAdding(true)
            }}
          >
            {t('wiki.tools.tags.add')}
          </button>
        )
      )}
    </div>
  )
}

import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { searchField } from '../wikiContext'

const isMac = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)

interface SearchBoxProps {
  value: string
  onChange: (value: string) => void
  /** listId and activeId connect the field to the list of results below it. */
  listId: string
  activeId: string | undefined
  expanded: boolean
  /** onMove steps through the results; onChoose opens the current one. */
  onMove: (step: 1 | -1) => void
  onChoose: () => void
  /** inputRef lets the tree put the cursor back after a tag is clicked. */
  inputRef: React.RefObject<HTMLInputElement | null>
}

/** SearchBox is the field at the top of the tree: ↑ ↓ Enter walk the results, Escape clears. */
export function SearchBox(p: SearchBoxProps) {
  const { t } = useTranslation()
  const { inputRef } = p

  // Ctrl+K, pressed anywhere in the wiki, ends up here.
  useEffect(() => {
    searchField.focus = () => {
      inputRef.current?.focus()
      inputRef.current?.select()
      searchField.pending = false
    }
    if (searchField.pending) searchField.focus()
    return () => {
      searchField.focus = null
    }
  }, [inputRef])

  return (
    <div className="search">
      <input
        ref={inputRef}
        type="search"
        className="search__input"
        role="combobox"
        aria-label={t('wiki.tools.search.label')}
        aria-expanded={p.expanded}
        aria-controls={p.listId}
        aria-activedescendant={p.activeId}
        aria-autocomplete="list"
        placeholder={t('wiki.tools.search.placeholder')}
        autoComplete="off"
        spellCheck={false}
        value={p.value}
        onChange={(e) => {
          p.onChange(e.target.value)
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault()
            p.onMove(e.key === 'ArrowDown' ? 1 : -1)
          } else if (e.key === 'Enter') {
            e.preventDefault()
            p.onChoose()
          } else if (e.key === 'Escape' && p.value !== '') {
            e.preventDefault()
            e.stopPropagation()
            p.onChange('')
          }
        }}
      />
      {p.value === '' && (
        <kbd className="search__hint" aria-hidden="true">
          {isMac ? '⌘ K' : t('wiki.tools.search.hint')}
        </kbd>
      )}
    </div>
  )
}

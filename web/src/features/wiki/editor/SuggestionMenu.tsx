import { useEffect, useRef } from 'react'

export interface MenuEntry {
  key: string
  label: string
  /** detail is shown on the right: a shortcut or the parent of the target. */
  detail?: string
  /** group is the heading above the first entry of a run with the same group. */
  group?: string
}

export interface SuggestionMenuProps {
  entries: MenuEntry[]
  selected: number
  loading: boolean
  emptyText: string
  label: string
  onSelect: (index: number) => void
}

/** SuggestionMenu is the panel of the "/" and "@" menus (docs/design/wiki-screen.md, "Меню и состояния"). */
export function SuggestionMenu({
  entries,
  selected,
  loading,
  emptyText,
  label,
  onSelect,
}: SuggestionMenuProps) {
  const selectedRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: 'nearest' })
  }, [selected, entries])

  if (entries.length === 0) {
    return loading ? null : (
      <div className="wiki-menu" role="status">
        <div className="wiki-menu__empty">{emptyText}</div>
      </div>
    )
  }

  return (
    <div className="wiki-menu" role="listbox" aria-label={label}>
      {entries.map((entry, i) => (
        <div key={entry.key}>
          {entry.group !== undefined && entry.group !== entries[i - 1]?.group && (
            <div className="wiki-menu__group">{entry.group}</div>
          )}
          <div
            ref={i === selected ? selectedRef : undefined}
            className="wiki-menu__item"
            role="option"
            aria-selected={i === selected}
            // The editor keeps the focus: the choice is made on mousedown.
            onMouseDown={(e) => {
              e.preventDefault()
              onSelect(i)
            }}
          >
            <span className="wiki-menu__label">{entry.label}</span>
            {entry.detail !== undefined && (
              <span className="wiki-menu__detail">{entry.detail}</span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

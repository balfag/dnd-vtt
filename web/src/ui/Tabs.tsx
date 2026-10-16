import { useRef } from 'react'
import { panelId, tabId } from './tabIds'

export interface Tab {
  id: string
  label: string
}

interface TabsProps {
  tabs: Tab[]
  selected: string
  onSelect: (id: string) => void
}

/** Tabs is a tab list; the caller renders the panels with role="tabpanel". */
export function Tabs({ tabs, selected, onSelect }: TabsProps) {
  const refs = useRef(new Map<string, HTMLButtonElement>())

  function move(from: number, step: number) {
    const next = tabs[(from + step + tabs.length) % tabs.length]
    if (next) {
      onSelect(next.id)
      refs.current.get(next.id)?.focus()
    }
  }

  return (
    <div className="tabs" role="tablist">
      {tabs.map((tab, i) => (
        <button
          key={tab.id}
          ref={(el) => {
            if (el) {
              refs.current.set(tab.id, el)
            } else {
              refs.current.delete(tab.id)
            }
          }}
          className="tabs__tab"
          type="button"
          role="tab"
          id={tabId(tab.id)}
          aria-selected={tab.id === selected}
          aria-controls={panelId(tab.id)}
          tabIndex={tab.id === selected ? 0 : -1}
          onClick={() => {
            onSelect(tab.id)
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
          {tab.label}
        </button>
      ))}
    </div>
  )
}

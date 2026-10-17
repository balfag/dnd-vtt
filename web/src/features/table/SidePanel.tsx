import { useRef, useState } from 'react'
import { panelId, tabId } from '../../ui/tabIds'

export interface SidePanelTab {
  id: string
  label: string
  content: React.ReactNode
}

const storageKey = 'dndvtt.sidePanelTab'

function remembered(): string | null {
  try {
    return window.localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

function remember(id: string) {
  try {
    window.localStorage.setItem(storageKey, id)
  } catch {
    // Storage may be blocked; the tab is a convenience, not state.
  }
}

interface SidePanelProps {
  tabs: SidePanelTab[]
  /** selected, when given, is the tab to show instead of the remembered one. */
  selected?: string
  onSelect?: (id: string) => void
}

/** SidePanel is the right-hand panel: a tab list and the content of the selected tab. */
export function SidePanel({ tabs, selected: requested, onSelect }: SidePanelProps) {
  const [chosen, setChosen] = useState<string | null>(remembered)
  const refs = useRef(new Map<string, HTMLButtonElement>())
  const selected = tabs.find((tab) => tab.id === (requested ?? chosen))?.id ?? tabs[0]?.id

  function select(id: string) {
    setChosen(id)
    remember(id)
    onSelect?.(id)
  }

  function move(from: number, step: number) {
    const next = tabs[(from + step + tabs.length) % tabs.length]
    if (next) {
      select(next.id)
      refs.current.get(next.id)?.focus()
    }
  }

  return (
    <aside className="sidepanel">
      <div className="sidepanel__tabs" role="tablist">
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
            className="sidepanel__tab"
            type="button"
            role="tab"
            id={tabId(tab.id)}
            aria-selected={tab.id === selected}
            aria-controls={panelId(tab.id)}
            tabIndex={tab.id === selected ? 0 : -1}
            onClick={() => {
              select(tab.id)
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
      {tabs.map((tab) => (
        <div
          key={tab.id}
          className="sidepanel__body"
          role="tabpanel"
          id={panelId(tab.id)}
          aria-labelledby={tabId(tab.id)}
          hidden={tab.id !== selected}
        >
          {tab.content}
        </div>
      ))}
    </aside>
  )
}

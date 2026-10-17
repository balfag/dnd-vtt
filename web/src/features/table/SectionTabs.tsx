import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

export type Section = 'map' | 'chat' | 'scenes'

interface SectionTabsProps {
  /** isGm adds the scenes section. */
  isGm: boolean
  selected: Section
  onSelect: (section: Section) => void
  /** unread is the number of chat messages that came while the map was shown. */
  unread: number
}

/** SectionTabs are the bottom tabs of a narrow screen: the map or the side panel. */
export function SectionTabs({ isGm, selected, onSelect, unread }: SectionTabsProps) {
  const { t } = useTranslation()
  const sections: Section[] = isGm ? ['map', 'chat', 'scenes'] : ['map', 'chat']
  const refs = useRef(new Map<Section, HTMLButtonElement>())

  function move(from: number, step: number) {
    const next = sections[(from + step + sections.length) % sections.length]
    if (next !== undefined) {
      onSelect(next)
      refs.current.get(next)?.focus()
    }
  }

  return (
    <div className="sections" role="tablist">
      {sections.map((section, i) => {
        const showDot = section === 'chat' && unread > 0
        return (
          <button
            key={section}
            ref={(el) => {
              if (el) {
                refs.current.set(section, el)
              } else {
                refs.current.delete(section)
              }
            }}
            className="sections__tab"
            type="button"
            role="tab"
            aria-selected={section === selected}
            aria-label={showDot ? t('table.section.chatUnread', { count: unread }) : undefined}
            tabIndex={section === selected ? 0 : -1}
            onClick={() => {
              onSelect(section)
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
            {section === 'scenes' ? t('scenes.tab') : t(`table.section.${section}`)}
            {showDot && <span className="sections__dot" aria-hidden="true" />}
          </button>
        )
      })}
    </div>
  )
}

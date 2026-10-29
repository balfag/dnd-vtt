import { useTranslation } from 'react-i18next'
import type { SearchHit } from '../../../shared/api/wiki'
import { hitId, segments } from './query'
import type { SearchState } from './useWikiSearch'

interface SearchResultsProps {
  id: string
  state: SearchState
  active: number
  onOpen: (hit: SearchHit) => void
  onHover: (index: number) => void
}

/** SearchResults replaces the tree while the field is not empty: icon, name, path and a snippet. */
export function SearchResults({ id, state, active, onOpen, onHover }: SearchResultsProps) {
  const { t } = useTranslation()

  if (state.status === 'noTag')
    return <p className="search__note">{t('wiki.tools.search.noTag')}</p>
  if (state.status === 'failed') {
    return <p className="search__note search__note--error">{t('wiki.tools.search.failed')}</p>
  }
  const hits = state.status === 'ready' || state.status === 'loading' ? state.hits : []
  if (state.status === 'ready' && hits.length === 0) {
    return (
      <p className="search__note" role="status">
        {t('wiki.tools.search.empty')}
      </p>
    )
  }

  return (
    <ul
      id={id}
      className="hits"
      role="listbox"
      aria-label={t('wiki.tools.search.results')}
      aria-busy={state.status === 'loading'}
    >
      {hits.map((hit, i) => (
        <li
          key={hit.id}
          id={hitId(id, i)}
          role="option"
          aria-selected={i === active}
          className={i === active ? 'hit hit--active' : 'hit'}
          onMouseMove={() => {
            if (i !== active) onHover(i)
          }}
          onMouseDown={(e) => {
            // Keep the focus in the field.
            e.preventDefault()
          }}
          onClick={() => {
            onOpen(hit)
          }}
        >
          <span className="hit__icon" aria-hidden="true">
            {hit.icon !== '' ? hit.icon : hit.kind === 'database' ? '▦' : '◦'}
          </span>
          <span className="hit__main">
            <span className="hit__title">{hit.title === '' ? t('wiki.untitled') : hit.title}</span>
            {hit.path.length > 0 && <span className="hit__path">{hit.path.join(' › ')}</span>}
            <span className="hit__snippet">
              {segments(hit.snippet).map((part, k) =>
                part.match ? <mark key={k}>{part.text}</mark> : <span key={k}>{part.text}</span>,
              )}
            </span>
          </span>
        </li>
      ))}
    </ul>
  )
}

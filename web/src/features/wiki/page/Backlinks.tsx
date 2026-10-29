import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getBacklinks, type PageRef } from '../../../shared/api/wiki'
import { useWikiActions } from '../wikiContext'

/** Backlinks lists the pages that mention this one, under the text. */
export function Backlinks({ pageId, savedAt }: { pageId: string; savedAt: number }) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const [refs, setRefs] = useState<PageRef[]>([])

  // The list is read when the page opens and again after a save: the page may have just changed what it mentions.
  useEffect(() => {
    let cancelled = false
    getBacklinks(pageId).then(
      (list) => {
        if (!cancelled) setRefs(list)
      },
      () => undefined,
    )
    return () => {
      cancelled = true
    }
  }, [pageId, savedAt])

  if (refs.length === 0) return null
  return (
    <footer className="backlinks">
      <h3 className="backlinks__title">{t('wiki.page.backlinks')}</h3>
      <ul className="backlinks__list">
        {refs.map((ref) => (
          <li key={ref.id}>
            <button
              type="button"
              className="phead__link"
              onClick={() => {
                actions.open(ref.id)
              }}
            >
              {[ref.icon, ref.title === '' ? t('wiki.untitled') : ref.title]
                .filter((s) => s !== '')
                .join(' ')}
            </button>
          </li>
        ))}
      </ul>
    </footer>
  )
}

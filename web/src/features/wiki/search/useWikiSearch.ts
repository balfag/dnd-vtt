import { useEffect, useState } from 'react'
import { listTags, searchWiki, type SearchHit } from '../../../shared/api/wiki'
import { findTag, hasWords, parseQuery } from './query'

export type SearchState =
  | { status: 'idle' }
  | { status: 'loading'; hits: SearchHit[] }
  | { status: 'ready'; hits: SearchHit[] }
  | { status: 'noTag' }
  | { status: 'failed' }

type Settled = Exclude<SearchState, { status: 'idle' | 'loading' }>

const DEBOUNCE_MS = 200
const LIMIT = 20

/** useWikiSearch looks the query up 200 ms after the last keystroke; an empty query is "idle". */
export function useWikiSearch(campaignId: string, query: string): SearchState {
  const text = query.trim()
  // The answer is kept with the text it answers; for any other text the search is still running.
  const [answer, setAnswer] = useState<{ text: string; result: Settled }>({
    text: '',
    result: { status: 'ready', hits: [] },
  })

  useEffect(() => {
    if (text === '') return
    let stale = false
    const timer = setTimeout(() => {
      void (async (): Promise<Settled> => {
        try {
          const parsed = parseQuery(text)
          let tagId: string | undefined
          if (parsed.tag !== null) {
            const tag = findTag(await listTags(campaignId), parsed.tag)
            if (tag === undefined) return { status: 'noTag' }
            tagId = tag.id
          } else if (!hasWords(parsed.q)) {
            return { status: 'ready', hits: [] }
          }
          const hits = await searchWiki(campaignId, { q: parsed.q, tag: tagId, limit: LIMIT })
          return { status: 'ready', hits }
        } catch {
          return { status: 'failed' }
        }
      })().then((result) => {
        if (!stale) setAnswer({ text, result })
      })
    }, DEBOUNCE_MS)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [campaignId, text])

  if (text === '') return { status: 'idle' }
  if (answer.text === text) return answer.result
  // Keep the last hits on screen while the next ones are on their way.
  return { status: 'loading', hits: answer.result.status === 'ready' ? answer.result.hits : [] }
}

import { useEffect, useState } from 'react'
import { useWiki } from '../store'

/**
 * useToolsList reads a list of the tree's sections (tags, templates, trash) and reads it again
 * whenever the wiki says something in it may have changed.
 */
export function useToolsList<T>(load: () => Promise<T[]>): { items: T[]; failed: boolean } {
  const version = useWiki((s) => s.toolsVersion)
  const [state, setState] = useState<{ items: T[]; failed: boolean }>({ items: [], failed: false })

  useEffect(() => {
    let stale = false
    load().then(
      (items) => {
        if (!stale) setState({ items, failed: false })
      },
      () => {
        if (!stale) setState((s) => ({ ...s, failed: true }))
      },
    )
    return () => {
      stale = true
    }
    // load is a fresh function on every render; the list changes with the campaign and the version only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version])

  return state
}

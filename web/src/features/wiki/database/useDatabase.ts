import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../../../shared/api/client'
import { getDatabase, type Database } from '../../../shared/api/wiki'

export type DatabaseLoad =
  { status: 'loading' } | { status: 'failed'; gone: boolean } | { status: 'ready'; db: Database }

/**
 * useDatabase reads a database and keeps it for editing: update changes the copy in memory, reload
 * reads it again (quietly, so the table does not flash) and runs when the window gets the focus.
 */
export function useDatabase(pageId: string): {
  load: DatabaseLoad
  update: (fn: (db: Database) => Database) => void
  reload: (quiet?: boolean) => void
  /** version grows with every reading; other readers of the same data (relations) follow it. */
  version: number
} {
  const [load, setLoad] = useState<DatabaseLoad>({ status: 'loading' })
  const [version, setVersion] = useState(0)
  const alive = useRef(true)

  // report tells that a failure is shown; a quiet reading keeps what is on the screen.
  const fetchIt = useCallback(
    (report: boolean) => {
      getDatabase(pageId).then(
        (db) => {
          if (!alive.current) return
          setLoad({ status: 'ready', db })
          setVersion((v) => v + 1)
        },
        (err: unknown) => {
          if (!alive.current) return
          if (!report) return
          setLoad({ status: 'failed', gone: err instanceof ApiError && err.status === 404 })
        },
      )
    },
    [pageId],
  )

  useEffect(() => {
    alive.current = true
    fetchIt(true)
    return () => {
      alive.current = false
    }
  }, [fetchIt])

  useEffect(() => {
    const onFocus = () => {
      fetchIt(false)
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
    }
  }, [fetchIt])

  const update = useCallback((fn: (db: Database) => Database) => {
    setLoad((now) => (now.status === 'ready' ? { status: 'ready', db: fn(now.db) } : now))
  }, [])

  const reload = useCallback(
    (quiet = true) => {
      if (!quiet) setLoad({ status: 'loading' })
      fetchIt(!quiet)
    },
    [fetchIt],
  )

  return { load, update, reload, version }
}

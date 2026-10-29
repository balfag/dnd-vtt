import { ApiError } from '../../../shared/api/client'
import type { Saved, SavePage, saveWikiPage } from '../../../shared/api/wiki'

export type SaveState = 'saved' | 'saving' | 'dirty' | 'failed' | 'conflict'

export type Draft = Omit<SavePage, 'version' | 'force'>

export interface Autosaver {
  /** change takes the latest state of the page and saves it after a second of quiet. */
  change(next: Draft): void
  /** flush saves now (a panel closes, the page is left, the tab is hidden). keepalive lets the request outlive the page. */
  flush(opts?: { keepalive?: boolean }): Promise<void>
  /** overwrite saves the kept draft over the version of the other window ("keep mine"). */
  overwrite(): Promise<void>
  readonly state: SaveState
  /** error is what stopped the last save: an ApiError of the server, or whatever the network threw. */
  readonly error: unknown
  subscribe(fn: (s: SaveState) => void): () => void
}

export const DEBOUNCE_MS = 1000

/**
 * createAutosaver keeps one page saved: one request at a time, the changes made meanwhile go
 * in the next one with the version the last reply gave. A conflict (409) stops everything
 * until overwrite(); any other failure keeps the draft for flush().
 */
export function createAutosaver(
  pageId: string,
  version: number,
  save: typeof saveWikiPage,
  onSaved: (s: Saved) => void,
): Autosaver {
  let current = version
  let state: SaveState = 'saved'
  let error: unknown = undefined
  let pending: Draft | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let running: Promise<void> | null = null
  const listeners = new Set<(s: SaveState) => void>()

  function setState(next: SaveState) {
    if (next === state) return
    state = next
    for (const fn of [...listeners]) fn(next)
  }

  async function drain(force: boolean, keepalive: boolean): Promise<void> {
    let forced = force
    while (pending !== null) {
      const body = pending
      pending = null
      setState('saving')
      try {
        const request: SavePage = { ...body, version: current, ...(forced ? { force: true } : {}) }
        const saved = await (keepalive
          ? save(pageId, request, { keepalive: true })
          : save(pageId, request))
        current = saved.version
        forced = false
        error = undefined
        onSaved(saved)
      } catch (err) {
        // Keep the draft unless a newer one came in during the request.
        pending ??= body
        error = err
        setState(err instanceof ApiError && err.status === 409 ? 'conflict' : 'failed')
        return
      }
    }
    setState('saved')
  }

  function start(force: boolean, keepalive: boolean): Promise<void> {
    if (running !== null) return running
    const run = drain(force, keepalive).finally(() => {
      running = null
    })
    running = run
    return run
  }

  return {
    change(next) {
      if (state === 'conflict') return
      pending = next
      clearTimeout(timer)
      if (running !== null) return
      setState('dirty')
      timer = setTimeout(() => {
        void start(false, false)
      }, DEBOUNCE_MS)
    },
    flush(opts) {
      clearTimeout(timer)
      if (state === 'conflict') return Promise.resolve()
      if (running === null && pending === null) return Promise.resolve()
      return start(false, opts?.keepalive === true)
    },
    overwrite() {
      clearTimeout(timer)
      if (running !== null) return running
      if (pending === null) {
        setState('saved')
        return Promise.resolve()
      }
      // A conflict stopped the loop, so nothing runs: leave the conflict and send the draft by force.
      state = 'dirty'
      return start(true, false)
    },
    get state() {
      return state
    },
    get error() {
      return error
    },
    subscribe(fn) {
      listeners.add(fn)
      return () => {
        listeners.delete(fn)
      }
    },
  }
}

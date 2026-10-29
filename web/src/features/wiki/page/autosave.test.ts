import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../../shared/api/client'
import type { Saved, SavePage } from '../../../shared/api/wiki'
import { createAutosaver, type Draft, type SaveState } from './autosave'

const draft = (title: string): Draft => ({
  title,
  icon: '',
  coverAssetId: null,
  content: { type: 'doc', content: [{ type: 'paragraph' }] },
})

const saved = (version: number): Saved => ({ version, updatedAt: version, refs: {} })

/** deferred is a save call whose reply the test sends by hand. */
function deferred() {
  const calls: { id: string; body: SavePage; opts?: { keepalive?: boolean } }[] = []
  const replies: { ok: (s: Saved) => void; fail: (e: unknown) => void }[] = []
  const save = vi.fn((id: string, body: SavePage, opts?: { keepalive?: boolean }) => {
    calls.push({ id, body, opts })
    return new Promise<Saved>((ok, fail) => {
      replies.push({ ok, fail })
    })
  })
  return { save, calls, replies }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('createAutosaver', () => {
  it('waits a second of quiet and sends one request for two changes', async () => {
    const d = deferred()
    const onSaved = vi.fn()
    const a = createAutosaver('p1', 3, d.save, onSaved)
    a.change(draft('a'))
    await vi.advanceTimersByTimeAsync(600)
    a.change(draft('ab'))
    expect(a.state).toBe('dirty')
    await vi.advanceTimersByTimeAsync(999)
    expect(d.save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(d.save).toHaveBeenCalledTimes(1)
    expect(d.calls[0]).toMatchObject({ id: 'p1', body: { title: 'ab', version: 3 } })
    expect(a.state).toBe('saving')

    d.replies[0]?.ok(saved(4))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.state).toBe('saved')
    expect(onSaved).toHaveBeenCalledWith(saved(4))
  })

  it('sends a change made during a request after it, with the version of the reply', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    a.change(draft('one'))
    await vi.advanceTimersByTimeAsync(1000)
    a.change(draft('two'))
    a.change(draft('three'))
    expect(a.state).toBe('saving')
    expect(d.save).toHaveBeenCalledTimes(1)

    d.replies[0]?.ok(saved(4))
    await vi.advanceTimersByTimeAsync(0)
    expect(d.save).toHaveBeenCalledTimes(2)
    expect(d.calls[1]?.body).toMatchObject({ title: 'three', version: 4 })
    d.replies[1]?.ok(saved(5))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.state).toBe('saved')
  })

  it('goes to conflict on 409 and stays silent', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    a.change(draft('mine'))
    await vi.advanceTimersByTimeAsync(1000)
    d.replies[0]?.fail(new ApiError(409, 'conflict'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.state).toBe('conflict')

    a.change(draft('more'))
    await a.flush()
    await vi.advanceTimersByTimeAsync(5000)
    expect(d.save).toHaveBeenCalledTimes(1)
    expect(a.state).toBe('conflict')
  })

  it('overwrite sends the draft again with force', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    a.change(draft('mine'))
    await vi.advanceTimersByTimeAsync(1000)
    d.replies[0]?.fail(new ApiError(409, 'conflict'))
    await vi.advanceTimersByTimeAsync(0)

    const done = a.overwrite()
    expect(d.save).toHaveBeenCalledTimes(2)
    expect(d.calls[1]?.body).toMatchObject({ title: 'mine', version: 3, force: true })
    d.replies[1]?.ok(saved(9))
    await done
    expect(a.state).toBe('saved')

    a.change(draft('after'))
    const next = a.flush()
    expect(d.calls[2]?.body).toMatchObject({ version: 9 })
    d.replies[2]?.ok(saved(10))
    await next
    expect(d.calls[2]?.body.force).toBeUndefined()
  })

  it('fails on a network error and flush tries again', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    const states: SaveState[] = []
    a.subscribe((s) => states.push(s))
    a.change(draft('x'))
    await vi.advanceTimersByTimeAsync(1000)
    d.replies[0]?.fail(new ApiError(0, 'network'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.state).toBe('failed')
    expect(a.error).toEqual(new ApiError(0, 'network'))

    const again = a.flush()
    expect(d.save).toHaveBeenCalledTimes(2)
    expect(d.calls[1]?.body).toMatchObject({ title: 'x', version: 3 })
    d.replies[1]?.ok(saved(4))
    await again
    expect(a.state).toBe('saved')
    expect(a.error).toBeUndefined()
    expect(states).toEqual(['dirty', 'saving', 'failed', 'saving', 'saved'])
  })

  it('treats 400 and 500 as a failure, not a conflict', async () => {
    for (const status of [400, 500]) {
      const d = deferred()
      const a = createAutosaver('p1', 1, d.save, vi.fn())
      a.change(draft('x'))
      await vi.advanceTimersByTimeAsync(1000)
      d.replies[0]?.fail(new ApiError(status, status === 400 ? 'bad_request' : 'internal'))
      await vi.advanceTimersByTimeAsync(0)
      expect(a.state).toBe('failed')
    }
  })

  it('flush sends at once, with keepalive when asked, and does nothing when saved', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    await a.flush()
    expect(d.save).not.toHaveBeenCalled()

    a.change(draft('x'))
    const done = a.flush({ keepalive: true })
    expect(d.calls[0]?.opts).toEqual({ keepalive: true })
    d.replies[0]?.ok(saved(4))
    await done
    await vi.advanceTimersByTimeAsync(5000)
    expect(d.save).toHaveBeenCalledTimes(1)
  })

  it('keeps a newer change when the request of an older one fails', async () => {
    const d = deferred()
    const a = createAutosaver('p1', 3, d.save, vi.fn())
    a.change(draft('old'))
    await vi.advanceTimersByTimeAsync(1000)
    a.change(draft('new'))
    d.replies[0]?.fail(new ApiError(0, 'network'))
    await vi.advanceTimersByTimeAsync(0)
    const again = a.flush()
    expect(d.calls[1]?.body.title).toBe('new')
    d.replies[1]?.ok(saved(4))
    await again
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { debounceSearch } from './mentions'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('debounceSearch', () => {
  it('searches once for calls that come close together and answers all of them', async () => {
    const search = vi.fn((q: string) => Promise.resolve([q]))
    const find = debounceSearch(search, 150)
    const a = find('a')
    await vi.advanceTimersByTimeAsync(100)
    const b = find('ab')
    await vi.advanceTimersByTimeAsync(149)
    expect(search).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(search).toHaveBeenCalledTimes(1)
    expect(search).toHaveBeenCalledWith('ab')
    await expect(a).resolves.toEqual(['ab'])
    await expect(b).resolves.toEqual(['ab'])
  })

  it('passes a failure on to every waiting call', async () => {
    const find = debounceSearch(() => Promise.reject(new Error('down')), 150)
    const a = find('a').catch((e: unknown) => e)
    await vi.advanceTimersByTimeAsync(150)
    await expect(a).resolves.toEqual(new Error('down'))
  })
})

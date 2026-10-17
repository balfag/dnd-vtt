import { describe, expect, it } from 'vitest'
import { quickActionOfKey } from './tokenActions'

const gm = { focused: true, isGM: true, hasSelection: true }

describe('quickActionOfKey', () => {
  it('maps the quick keys of the GM on a selected token', () => {
    expect(quickActionOfKey('Enter', gm)).toBe('properties')
    expect(quickActionOfKey('h', gm)).toBe('toggleHidden')
    expect(quickActionOfKey('L', gm)).toBe('toggleLocked')
    expect(quickActionOfKey(']', gm)).toBe('toFront')
    expect(quickActionOfKey('[', gm)).toBe('toBack')
    expect(quickActionOfKey('Delete', gm)).toBe('delete')
  })

  it('does not depend on the active tool, so it works right after placing a token', () => {
    // The context carries no tool at all: the place tool in hand changes nothing.
    expect(quickActionOfKey('Enter', gm)).toBe('properties')
  })

  it('does nothing without focus, selection or GM rights', () => {
    expect(quickActionOfKey('Enter', { ...gm, focused: false })).toBeNull()
    expect(quickActionOfKey('Enter', { ...gm, hasSelection: false })).toBeNull()
    expect(quickActionOfKey('Enter', { ...gm, isGM: false })).toBeNull()
    expect(quickActionOfKey('x', gm)).toBeNull()
  })
})

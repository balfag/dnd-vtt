import { describe, expect, it } from 'vitest'
import { filterSlashItems, slashItems } from './slashItems'
import { groupByKind } from './mentionSuggestion'
import { isAllowedHref, isValidEmoji, isValidLanguage, normalizeHref } from './rules'

describe('isValidEmoji', () => {
  it.each(['💡', '🍺', 'a', '👨‍👩‍👧', '12345678'])('%s is valid', (s) => {
    expect(isValidEmoji(s)).toBe(true)
  })

  it.each(['', '123456789', 'a\nb', '\u0000', '\ud800'])('%j is not', (s) => {
    expect(isValidEmoji(s)).toBe(false)
  })
})

describe('isValidLanguage', () => {
  it.each(['js', 'c++', 'c#', 'objective-c', 'a'.repeat(20)])('%s is valid', (s) => {
    expect(isValidLanguage(s)).toBe(true)
  })

  it.each(['', 'JS', 'a'.repeat(21), 'a b', null, 5])('%j is not', (s) => {
    expect(isValidLanguage(s)).toBe(false)
  })
})

describe('links', () => {
  it('allows http, https and mailto only', () => {
    expect(isAllowedHref('https://example.com')).toBe(true)
    expect(isAllowedHref('HTTP://example.com')).toBe(true)
    expect(isAllowedHref('mailto:a@b.c')).toBe(true)
    for (const bad of [
      'javascript:alert(1)',
      'data:text/html,x',
      'ftp://x.y',
      '/a/b',
      'example.com',
    ]) {
      expect(isAllowedHref(bad), bad).toBe(false)
    }
  })

  it('adds https:// to a bare address and refuses other schemes', () => {
    expect(normalizeHref(' example.com/a ')).toBe('https://example.com/a')
    expect(normalizeHref('mailto:a@b.c')).toBe('mailto:a@b.c')
    expect(normalizeHref('javascript:alert(1)')).toBeNull()
    expect(normalizeHref('   ')).toBeNull()
  })
})

describe('slash menu search', () => {
  const title = (item: { id: string }) =>
    ({ callout: 'Выноска', table: 'Таблица' })[item.id] ?? item.id
  const ids = (query: string, canUpload = true) =>
    filterSlashItems(slashItems, query, title, canUpload).map((i) => i.id)

  it('lists every item for an empty query, in menu order', () => {
    expect(ids('')).toEqual(slashItems.map((i) => i.id))
  })

  it('searches by the translated title and by an English keyword', () => {
    expect(ids('выно')).toEqual(['callout'])
    expect(ids('aside')).toEqual(['callout'])
    expect(ids('grid')).toEqual(['table'])
  })

  it('offers the image only when images can be uploaded', () => {
    expect(ids('image', true)).toEqual(['image'])
    expect(ids('image', false)).toEqual([])
    expect(ids('', false)).not.toContain('image')
  })
})

describe('groupByKind', () => {
  it('orders pages, scenes, tokens and keeps the order inside a kind', () => {
    const item = (kind: 'page' | 'scene' | 'token', id: string) => ({
      kind,
      id,
      label: id,
      detail: '',
    })
    const items = [item('token', 't1'), item('page', 'p1'), item('scene', 's1'), item('page', 'p2')]
    expect(groupByKind(items).map((i) => i.id)).toEqual(['p1', 'p2', 's1', 't1'])
  })
})

import { describe, expect, it } from 'vitest'
import { findTag, hasWords, isSearchShortcut, parseQuery, segments, suggestTags } from './query'

describe('parseQuery', () => {
  it('reads a tag in the first word', () => {
    expect(parseQuery('#тег слова')).toEqual({ tag: 'тег', q: 'слова' })
    expect(parseQuery('  #NPC  ')).toEqual({ tag: 'NPC', q: '' })
  })

  it('does not read a tag after the first word', () => {
    expect(parseQuery('слова #тег')).toEqual({ tag: null, q: 'слова #тег' })
  })

  it('treats a lone # as text', () => {
    expect(parseQuery('#')).toEqual({ tag: null, q: '#' })
  })
})

describe('segments', () => {
  it('marks the matches and drops empty pieces', () => {
    expect(
      segments([{ text: 'Хозяин — ' }, { text: 'Барлиман', match: true }, { text: '' }]),
    ).toEqual([
      { text: 'Хозяин — ', match: false },
      { text: 'Барлиман', match: true },
    ])
  })

  it('keeps markup as text', () => {
    expect(segments([{ text: '<b>x</b>', match: true }])).toEqual([
      { text: '<b>x</b>', match: true },
    ])
  })
})

describe('findTag', () => {
  it('ignores case', () => {
    expect(findTag([{ name: 'Таверна' }], 'таверна')).toEqual({ name: 'Таверна' })
    expect(findTag([{ name: 'Таверна' }], 'нет')).toBeUndefined()
  })
})

describe('isSearchShortcut', () => {
  const key = { key: 'k', code: 'KeyK', ctrlKey: true, metaKey: false, altKey: false }

  it('takes Ctrl+K, Cmd+K and the Russian layout', () => {
    expect(isSearchShortcut(key)).toBe(true)
    expect(isSearchShortcut({ ...key, ctrlKey: false, metaKey: true })).toBe(true)
    expect(isSearchShortcut({ ...key, key: 'л' })).toBe(true)
  })

  it('leaves the plain key and Alt alone', () => {
    expect(isSearchShortcut({ ...key, ctrlKey: false })).toBe(false)
    expect(isSearchShortcut({ ...key, altKey: true })).toBe(false)
  })
})

describe('hasWords', () => {
  it('needs a letter or a digit', () => {
    expect(hasWords('таверна')).toBe(true)
    expect(hasWords('9')).toBe(true)
    expect(hasWords('# - ?')).toBe(false)
  })
})

describe('suggestTags', () => {
  it('matches the prefix without case and skips the tags the page has', () => {
    const known = ['Таверна', 'таинственный', 'NPC', 'Тракт']
    expect(suggestTags(known, ['Таверна'], 'та')).toEqual(['таинственный'])
    expect(suggestTags(known, [], 'ТР')).toEqual(['Тракт'])
    expect(suggestTags(known, [], '')).toEqual(known)
  })
})

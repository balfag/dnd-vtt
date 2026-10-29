import { describe, expect, it } from 'vitest'
import { findTag, parseQuery, segments } from './query'

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

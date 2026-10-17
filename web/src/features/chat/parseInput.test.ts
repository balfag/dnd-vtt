import { describe, expect, it } from 'vitest'
import { parseInput } from './parseInput'

const players = [
  { id: 'p1', displayName: 'Мастер' },
  { id: 'p2', displayName: 'Anna' },
  { id: 'p3', displayName: 'Boris' },
  { id: 'p4', displayName: 'Boris' },
]
const me = 'p2'

function parse(input: string) {
  return parseInput(input, players, me)
}

describe('parseInput text', () => {
  it('trims a plain message', () => {
    expect(parse('  hello there  ')).toEqual({ kind: 'text', text: 'hello there' })
  })

  it('rejects an empty message', () => {
    expect(parse('')).toEqual({ kind: 'error', key: 'chat.error.empty' })
    expect(parse('   ')).toEqual({ kind: 'error', key: 'chat.error.empty' })
  })

  it('unescapes a double slash', () => {
    expect(parse('//shrug')).toEqual({ kind: 'text', text: '/shrug' })
    expect(parse('//r 1d20')).toEqual({ kind: 'text', text: '/r 1d20' })
  })

  it('keeps a slash in the middle', () => {
    expect(parse('and/or')).toEqual({ kind: 'text', text: 'and/or' })
  })
})

describe('parseInput rolls', () => {
  it.each([
    ['/r', 'public'],
    ['/gmr', 'gm'],
    ['/sr', 'self'],
    ['/br', 'blind'],
  ] as const)('%s is a %s roll', (command, visibility) => {
    expect(parse(`${command} 2d20kh1+5`)).toEqual({
      kind: 'roll',
      formula: '2d20kh1+5',
      label: '',
      visibility,
    })
  })

  it('takes the rest as a label', () => {
    expect(parse('/r 1d20+3   Perception check ')).toEqual({
      kind: 'roll',
      formula: '1d20+3',
      label: 'Perception check',
      visibility: 'public',
    })
  })

  it('does not check the formula', () => {
    expect(parse('/r nonsense')).toMatchObject({ kind: 'roll', formula: 'nonsense' })
  })

  it('needs a formula', () => {
    expect(parse('/r')).toEqual({ kind: 'error', key: 'chat.error.noFormula' })
    expect(parse('/gmr   ')).toEqual({ kind: 'error', key: 'chat.error.noFormula' })
  })

  it('accepts a label of exactly 100 characters', () => {
    const label = 'a'.repeat(100)
    expect(parse(`/r 1d6 ${label}`)).toMatchObject({ kind: 'roll', label })
  })

  it('rejects a label of 101 characters', () => {
    expect(parse(`/r 1d6 ${'a'.repeat(101)}`)).toEqual({
      kind: 'error',
      key: 'chat.error.labelTooLong',
    })
  })

  it('knows the command in any case', () => {
    expect(parse('/R 1d4')).toMatchObject({ kind: 'roll', visibility: 'public' })
  })
})

describe('parseInput whispers', () => {
  it('finds one participant', () => {
    expect(parse('/w Boris: hi')).toEqual({ kind: 'private', to: ['p3', 'p4'], text: 'hi' })
  })

  it('finds several participants', () => {
    expect(parse('/w Мастер, boris: psst')).toEqual({
      kind: 'private',
      to: ['p1', 'p3', 'p4'],
      text: 'psst',
    })
  })

  it('ignores case and spaces around names', () => {
    expect(parse('/w   МАСТЕР  ,  boris :  x ')).toEqual({
      kind: 'private',
      to: ['p1', 'p3', 'p4'],
      text: 'x',
    })
    expect(parse('/w   мастер  :  x ')).toEqual({ kind: 'private', to: ['p1'], text: 'x' })
  })

  it('addresses both participants with the same name', () => {
    const got = parse('/w boris: hi')
    expect(got).toEqual({ kind: 'private', to: ['p3', 'p4'], text: 'hi' })
  })

  it('does not repeat an id for a repeated name', () => {
    expect(parse('/w Мастер, мастер: hi')).toEqual({ kind: 'private', to: ['p1'], text: 'hi' })
  })

  it('keeps colons in the text', () => {
    expect(parse('/w Мастер: time: 5')).toEqual({ kind: 'private', to: ['p1'], text: 'time: 5' })
  })

  it('rejects an unknown name', () => {
    expect(parse('/w Carl: hi')).toEqual({
      kind: 'error',
      key: 'chat.error.noPlayer',
      params: { name: 'Carl' },
    })
  })

  it('rejects the author', () => {
    expect(parse('/w Anna: hi')).toEqual({ kind: 'error', key: 'chat.error.self' })
    expect(parse('/w Мастер, anna: hi')).toEqual({ kind: 'error', key: 'chat.error.self' })
  })

  it('rejects a broken format', () => {
    const bad = { kind: 'error', key: 'chat.error.whisperFormat' }
    expect(parse('/w')).toEqual(bad)
    expect(parse('/w Boris hi')).toEqual(bad)
    expect(parse('/w Boris:')).toEqual(bad)
    expect(parse('/w Boris:   ')).toEqual(bad)
    expect(parse('/w : hi')).toEqual(bad)
  })
})

describe('parseInput commands', () => {
  it('rejects an unknown command', () => {
    expect(parse('/dance now')).toEqual({ kind: 'error', key: 'chat.error.unknownCommand' })
    expect(parse('/')).toEqual({ kind: 'error', key: 'chat.error.unknownCommand' })
    expect(parse('/constructor')).toEqual({ kind: 'error', key: 'chat.error.unknownCommand' })
  })
})

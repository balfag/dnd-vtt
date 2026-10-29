import { describe, expect, it } from 'vitest'
import type { DbRecord, Filter, Property, PropertyType, ViewConfig } from '../../../shared/api/wiki'
import { applyView, fieldOps, localDate, propertyOps } from './filter'

const props: Property[] = [
  { id: 'text', name: 'Text', type: 'text', position: 0, options: {} },
  { id: 'url', name: 'Url', type: 'url', position: 1, options: {} },
  { id: 'num', name: 'Number', type: 'number', position: 2, options: {} },
  {
    id: 'sel',
    name: 'Select',
    type: 'select',
    position: 3,
    options: {
      options: [
        { id: 'a', name: 'alive', color: 'green' },
        { id: 'd', name: 'dead', color: 'red' },
      ],
    },
  },
  {
    id: 'multi',
    name: 'Multi',
    type: 'multi_select',
    position: 4,
    options: {
      options: [
        { id: 'x', name: 'x', color: 'gray' },
        { id: 'y', name: 'y', color: 'blue' },
      ],
    },
  },
  { id: 'chk', name: 'Check', type: 'checkbox', position: 5, options: {} },
  { id: 'date', name: 'Date', type: 'date', position: 6, options: {} },
  {
    id: 'rel',
    name: 'Relation',
    type: 'relation',
    position: 7,
    options: { targetDatabaseId: 'db' },
  },
  { id: 'link', name: 'Link', type: 'game_link', position: 8, options: {} },
]

const day = (y: number, m: number, d: number) => new Date(y, m - 1, d, 12).getTime()

function rec(
  id: string,
  title: string,
  values: Record<string, unknown> = {},
  extra: Partial<DbRecord> = {},
): DbRecord {
  return {
    id,
    title,
    icon: '',
    coverAssetId: null,
    props: values,
    tags: [],
    createdAt: day(2026, 1, 10),
    updatedAt: day(2026, 1, 20),
    ...extra,
  }
}

const config = (over: Partial<ViewConfig>): ViewConfig => ({
  filters: [],
  sorts: [],
  visible: [],
  widths: {},
  ...over,
})

const tags = new Map<string, string>([['t1', 'NPC']])

function ids(records: DbRecord[], filter: Filter): string[] {
  return applyView(records, props, config({ filters: [filter] }), tags).map((r) => r.id)
}

describe('filters by property', () => {
  it('text contains, ignoring case', () => {
    const rs = [
      rec('1', '', { text: 'Старый ДРАКОН' }),
      rec('2', '', { text: 'эльф' }),
      rec('3', ''),
    ]
    expect(ids(rs, { propertyId: 'text', op: 'contains', value: 'дракон' })).toEqual(['1'])
  })

  it('text empty and not_empty; an absent key and "" are empty', () => {
    const rs = [rec('1', '', { text: 'a' }), rec('2', '', { text: '' }), rec('3', '')]
    expect(ids(rs, { propertyId: 'text', op: 'empty' })).toEqual(['2', '3'])
    expect(ids(rs, { propertyId: 'text', op: 'not_empty' })).toEqual(['1'])
  })

  it('url works like text', () => {
    const rs = [rec('1', '', { url: 'https://Example.com/x' }), rec('2', '')]
    expect(ids(rs, { propertyId: 'url', op: 'contains', value: 'example' })).toEqual(['1'])
    expect(ids(rs, { propertyId: 'url', op: 'empty' })).toEqual(['2'])
    expect(ids(rs, { propertyId: 'url', op: 'not_empty' })).toEqual(['1'])
  })

  it.each([
    ['eq', 5, ['b']],
    ['neq', 5, ['a', 'c']],
    ['lt', 5, ['a']],
    ['lte', 5, ['a', 'b']],
    ['gt', 5, ['c']],
    ['gte', 5, ['b', 'c']],
  ])('number %s', (op, value, want) => {
    const rs = [
      rec('a', '', { num: 2 }),
      rec('b', '', { num: 5 }),
      rec('c', '', { num: 10 }),
      rec('d', ''),
    ]
    expect(ids(rs, { propertyId: 'num', op, value })).toEqual(want)
  })

  it('number compares numbers, not strings, and empty finds the missing', () => {
    const rs = [rec('a', '', { num: 9 }), rec('b', '', { num: 10 }), rec('c', '')]
    expect(ids(rs, { propertyId: 'num', op: 'gt', value: 9 })).toEqual(['b'])
    expect(ids(rs, { propertyId: 'num', op: 'empty' })).toEqual(['c'])
  })

  it('select eq, neq (the unset pass neq) and empty', () => {
    const rs = [rec('1', '', { sel: 'a' }), rec('2', '', { sel: 'd' }), rec('3', '')]
    expect(ids(rs, { propertyId: 'sel', op: 'eq', value: 'd' })).toEqual(['2'])
    expect(ids(rs, { propertyId: 'sel', op: 'neq', value: 'd' })).toEqual(['1', '3'])
    expect(ids(rs, { propertyId: 'sel', op: 'empty' })).toEqual(['3'])
  })

  it('multi_select contains, not_contains and empty', () => {
    const rs = [
      rec('1', '', { multi: ['x', 'y'] }),
      rec('2', '', { multi: ['y'] }),
      rec('3', '', { multi: [] }),
      rec('4', ''),
    ]
    expect(ids(rs, { propertyId: 'multi', op: 'contains', value: 'x' })).toEqual(['1'])
    expect(ids(rs, { propertyId: 'multi', op: 'not_contains', value: 'x' })).toEqual([
      '2',
      '3',
      '4',
    ])
    expect(ids(rs, { propertyId: 'multi', op: 'empty' })).toEqual(['3', '4'])
  })

  it('checkbox checked and unchecked; an absent value is unchecked', () => {
    const rs = [rec('1', '', { chk: true }), rec('2', '', { chk: false }), rec('3', '')]
    expect(ids(rs, { propertyId: 'chk', op: 'checked' })).toEqual(['1'])
    expect(ids(rs, { propertyId: 'chk', op: 'unchecked' })).toEqual(['2', '3'])
  })

  it('date before, after, eq and empty', () => {
    const rs = [
      rec('1', '', { date: '2026-03-01' }),
      rec('2', '', { date: '2026-03-15' }),
      rec('3', ''),
    ]
    expect(ids(rs, { propertyId: 'date', op: 'before', value: '2026-03-15' })).toEqual(['1'])
    expect(ids(rs, { propertyId: 'date', op: 'after', value: '2026-03-01' })).toEqual(['2'])
    expect(ids(rs, { propertyId: 'date', op: 'eq', value: '2026-03-15' })).toEqual(['2'])
    expect(ids(rs, { propertyId: 'date', op: 'empty' })).toEqual(['3'])
  })

  it.each(['rel', 'link'])('%s empty and not_empty', (id) => {
    const rs = [
      rec('1', '', { [id]: [{ kind: 'scene', id: 's' }] }),
      rec('2', '', { [id]: [] }),
      rec('3', ''),
    ]
    expect(ids(rs, { propertyId: id, op: 'empty' })).toEqual(['2', '3'])
    expect(ids(rs, { propertyId: id, op: 'not_empty' })).toEqual(['1'])
  })

  it('a filter of an unknown property lets everything pass', () => {
    const rs = [rec('1', ''), rec('2', '')]
    expect(ids(rs, { propertyId: 'gone', op: 'empty' })).toEqual(['1', '2'])
  })
})

describe('filters by field', () => {
  it('title contains, empty, not_empty', () => {
    const rs = [rec('1', 'Барлиман'), rec('2', 'Мирта'), rec('3', '')]
    expect(ids(rs, { field: 'title', op: 'contains', value: 'БАРЛ' })).toEqual(['1'])
    expect(ids(rs, { field: 'title', op: 'empty' })).toEqual(['3'])
    expect(ids(rs, { field: 'title', op: 'not_empty' })).toEqual(['1', '2'])
  })

  it('createdAt and updatedAt compare the local date', () => {
    const rs = [
      rec('1', '', {}, { createdAt: day(2026, 2, 1), updatedAt: day(2026, 5, 1) }),
      rec('2', '', {}, { createdAt: day(2026, 3, 1), updatedAt: day(2026, 4, 1) }),
    ]
    expect(ids(rs, { field: 'createdAt', op: 'before', value: '2026-03-01' })).toEqual(['1'])
    expect(ids(rs, { field: 'createdAt', op: 'after', value: '2026-02-01' })).toEqual(['2'])
    expect(ids(rs, { field: 'updatedAt', op: 'eq', value: '2026-04-01' })).toEqual(['2'])
    expect(localDate(day(2026, 4, 1))).toBe('2026-04-01')
  })

  it('tags contains finds a tag by id, or by name after a merge', () => {
    const rs = [
      rec('1', '', {}, { tags: [{ id: 't1', name: 'NPC' }] }),
      rec('2', '', {}, { tags: [{ id: 't2', name: 'npc' }] }),
      rec('3', '', {}, { tags: [{ id: 't3', name: 'орк' }] }),
    ]
    expect(ids(rs, { field: 'tags', op: 'contains', value: 't1' })).toEqual(['1', '2'])
    expect(ids(rs, { field: 'tags', op: 'contains', value: 't3' })).toEqual(['3'])
  })
})

describe('applyView', () => {
  it('joins filters with AND', () => {
    const rs = [
      rec('1', 'a', { sel: 'a', num: 1 }),
      rec('2', 'b', { sel: 'a', num: 9 }),
      rec('3', 'c', { sel: 'd', num: 9 }),
    ]
    const out = applyView(
      rs,
      props,
      config({
        filters: [
          { propertyId: 'sel', op: 'eq', value: 'a' },
          { propertyId: 'num', op: 'gt', value: 5 },
        ],
      }),
      tags,
    )
    expect(out.map((r) => r.id)).toEqual(['2'])
  })

  it('sorts by two keys', () => {
    const rs = [
      rec('1', 'b', { sel: 'd' }),
      rec('2', 'a', { sel: 'd' }),
      rec('3', 'c', { sel: 'a' }),
    ]
    const out = applyView(
      rs,
      props,
      config({
        sorts: [
          { propertyId: 'sel', dir: 'desc' },
          { field: 'title', dir: 'asc' },
        ],
      }),
      tags,
    )
    expect(out.map((r) => r.id)).toEqual(['2', '1', '3'])
  })

  it('puts the empty last under asc and desc', () => {
    const rs = [rec('1', '', { num: 3 }), rec('2', ''), rec('3', '', { num: 1 })]
    const asc = applyView(rs, props, config({ sorts: [{ propertyId: 'num', dir: 'asc' }] }), tags)
    const desc = applyView(rs, props, config({ sorts: [{ propertyId: 'num', dir: 'desc' }] }), tags)
    expect(asc.map((r) => r.id)).toEqual(['3', '1', '2'])
    expect(desc.map((r) => r.id)).toEqual(['1', '3', '2'])
  })

  it('sorts a Cyrillic title by the alphabet', () => {
    const rs = [rec('1', 'Яна'), rec('2', 'Барлиман'), rec('3', 'Ёжик'), rec('4', 'Мирта')]
    const out = applyView(rs, props, config({ sorts: [{ field: 'title', dir: 'asc' }] }), tags)
    expect(out.map((r) => r.title)).toEqual(['Барлиман', 'Ёжик', 'Мирта', 'Яна'])
  })

  it('sorts by the order of the variants of a select', () => {
    const rs = [rec('1', '', { sel: 'd' }), rec('2', '', { sel: 'a' })]
    const out = applyView(rs, props, config({ sorts: [{ propertyId: 'sel', dir: 'asc' }] }), tags)
    expect(out.map((r) => r.id)).toEqual(['2', '1'])
  })

  it('does not change the list it is given', () => {
    const rs = [rec('1', 'b'), rec('2', 'a')]
    applyView(rs, props, config({ sorts: [{ field: 'title', dir: 'asc' }] }), tags)
    expect(rs.map((r) => r.id)).toEqual(['1', '2'])
  })
})

describe('operator tables', () => {
  it('cover every property type and field the server knows', () => {
    const types: PropertyType[] = props.map((p) => p.type)
    expect(Object.keys(propertyOps).sort()).toEqual([...types].sort())
    expect(Object.keys(fieldOps).sort()).toEqual(['createdAt', 'tags', 'title', 'updatedAt'])
  })
})

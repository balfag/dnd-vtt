import i18n from 'i18next'
import type {
  DbRecord,
  Filter,
  Property,
  PropertyType,
  SortKey,
  ViewConfig,
} from '../../../shared/api/wiki'

/** valueKind is what a filter operator takes as its value; it mirrors internal/wiki/viewconfig.go. */
export type ValueKind = 'none' | 'text' | 'number' | 'option' | 'date' | 'tag'

const textOps: Record<string, ValueKind> = { contains: 'text', empty: 'none', not_empty: 'none' }
const linkOps: Record<string, ValueKind> = { empty: 'none', not_empty: 'none' }
const dateFieldOps: Record<string, ValueKind> = { before: 'date', after: 'date', eq: 'date' }

/** propertyOps lists the operators of each property type, in the order they are offered. */
export const propertyOps: Record<PropertyType, Record<string, ValueKind>> = {
  text: textOps,
  url: textOps,
  number: {
    eq: 'number',
    neq: 'number',
    lt: 'number',
    lte: 'number',
    gt: 'number',
    gte: 'number',
    empty: 'none',
  },
  select: { eq: 'option', neq: 'option', empty: 'none' },
  multi_select: { contains: 'option', not_contains: 'option', empty: 'none' },
  checkbox: { checked: 'none', unchecked: 'none' },
  date: { before: 'date', after: 'date', eq: 'date', empty: 'none' },
  relation: linkOps,
  game_link: linkOps,
}

/** fieldOps lists the operators of the built-in fields. */
export const fieldOps: Record<string, Record<string, ValueKind>> = {
  title: textOps,
  createdAt: dateFieldOps,
  updatedAt: dateFieldOps,
  tags: { contains: 'tag' },
}

export const sortFields = ['title', 'createdAt', 'updatedAt'] as const

/** isEmpty: the value is absent, or an empty string or list. */
export function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
}

function lower(s: string): string {
  return s.toLocaleLowerCase()
}

/** localDate is the YYYY-MM-DD date of a moment in the time zone of the browser. */
export function localDate(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function compareDate(actual: string | undefined, op: string, want: unknown): boolean {
  if (actual === undefined || typeof want !== 'string') return false
  if (op === 'before') return actual < want
  if (op === 'after') return actual > want
  return actual === want
}

function matchText(actual: unknown, op: string, want: unknown): boolean {
  const text = typeof actual === 'string' ? actual : ''
  switch (op) {
    case 'contains':
      return typeof want === 'string' && lower(text).includes(lower(want))
    case 'empty':
      return text === ''
    case 'not_empty':
      return text !== ''
    default:
      return true
  }
}

function matchNumber(actual: unknown, op: string, want: unknown): boolean {
  if (op === 'empty') return isEmpty(actual)
  if (typeof actual !== 'number' || typeof want !== 'number') return false
  switch (op) {
    case 'eq':
      return actual === want
    case 'neq':
      return actual !== want
    case 'lt':
      return actual < want
    case 'lte':
      return actual <= want
    case 'gt':
      return actual > want
    case 'gte':
      return actual >= want
    default:
      return true
  }
}

function matchProperty(record: DbRecord, prop: Property, f: Filter): boolean {
  const actual = record.props[prop.id]
  switch (prop.type) {
    case 'text':
    case 'url':
      return matchText(actual, f.op, f.value)
    case 'number':
      return matchNumber(actual, f.op, f.value)
    case 'select':
      if (f.op === 'empty') return isEmpty(actual)
      return f.op === 'eq' ? actual === f.value : actual !== f.value
    case 'multi_select': {
      if (f.op === 'empty') return isEmpty(actual)
      const has = Array.isArray(actual) && actual.includes(f.value)
      return f.op === 'contains' ? has : !has
    }
    case 'checkbox':
      return f.op === 'checked' ? actual === true : actual !== true
    case 'date':
      if (f.op === 'empty') return isEmpty(actual)
      return compareDate(typeof actual === 'string' ? actual : undefined, f.op, f.value)
    case 'relation':
    case 'game_link':
      return f.op === 'empty' ? isEmpty(actual) : !isEmpty(actual)
  }
}

function matchField(record: DbRecord, f: Filter, tags: Map<string, string>): boolean {
  switch (f.field) {
    case 'title':
      return matchText(record.title, f.op, f.value)
    case 'createdAt':
      return compareDate(localDate(record.createdAt), f.op, f.value)
    case 'updatedAt':
      return compareDate(localDate(record.updatedAt), f.op, f.value)
    case 'tags': {
      if (typeof f.value !== 'string') return false
      const name = tags.get(f.value)
      return record.tags.some(
        (t) => t.id === f.value || (name !== undefined && lower(t.name) === lower(name)),
      )
    }
    default:
      return true
  }
}

/** matches tells whether a record passes one filter; a filter of a property that is gone passes everything. */
function matches(
  record: DbRecord,
  byId: Map<string, Property>,
  f: Filter,
  tags: Map<string, string>,
): boolean {
  if (f.propertyId !== undefined) {
    const prop = byId.get(f.propertyId)
    return prop === undefined ? true : matchProperty(record, prop, f)
  }
  return matchField(record, f, tags)
}

/** sortValue is what a record is compared by under a key; undefined is "empty" and goes last. */
function sortValue(
  record: DbRecord,
  prop: Property | undefined,
  key: SortKey,
): string | number | undefined {
  if (key.propertyId === undefined) {
    if (key.field === 'createdAt') return record.createdAt
    if (key.field === 'updatedAt') return record.updatedAt
    return record.title === '' ? undefined : record.title
  }
  if (prop === undefined) return undefined
  const v = record.props[prop.id]
  switch (prop.type) {
    case 'checkbox':
      return v === true ? 1 : 0
    case 'select': {
      if (typeof v !== 'string') return undefined
      const at = (prop.options.options ?? []).findIndex((o) => o.id === v)
      return at < 0 ? undefined : at
    }
    case 'multi_select': {
      if (!Array.isArray(v) || v.length === 0) return undefined
      const names = v.map((id) => (prop.options.options ?? []).find((o) => o.id === id)?.name ?? '')
      return names.join(' ')
    }
    case 'relation':
    case 'game_link':
      return Array.isArray(v) && v.length > 0 ? v.length : undefined
    case 'number':
      return typeof v === 'number' ? v : undefined
    default:
      return typeof v === 'string' && v !== '' ? v : undefined
  }
}

function compareValues(a: string | number, b: string | number): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b
  return String(a).localeCompare(String(b), i18n.language || undefined)
}

/**
 * applyView filters the records by all the filters of the view (AND) and sorts them by its keys.
 * Strings are compared without case; empty values go last under either direction.
 * tags maps a tag id to its name.
 */
export function applyView(
  records: DbRecord[],
  props: Property[],
  config: ViewConfig,
  tags: Map<string, string>,
): DbRecord[] {
  const byId = new Map(props.map((p) => [p.id, p]))
  let out =
    config.filters.length === 0
      ? records.slice()
      : records.filter((r) => config.filters.every((f) => matches(r, byId, f, tags)))
  if (config.sorts.length === 0) return out
  const keys = config.sorts.map((key) => ({
    key,
    prop: key.propertyId === undefined ? undefined : byId.get(key.propertyId),
  }))
  const cache = new Map<DbRecord, (string | number | undefined)[]>()
  const valuesOf = (r: DbRecord) => {
    let v = cache.get(r)
    if (v === undefined) {
      v = keys.map(({ key, prop }) => sortValue(r, prop, key))
      cache.set(r, v)
    }
    return v
  }
  out = out.sort((x, y) => {
    const xs = valuesOf(x)
    const ys = valuesOf(y)
    for (const [i, { key }] of keys.entries()) {
      const a = xs[i]
      const b = ys[i]
      if (a === undefined && b === undefined) continue
      if (a === undefined) return 1
      if (b === undefined) return -1
      const c = compareValues(a, b)
      if (c !== 0) return key.dir === 'asc' ? c : -c
    }
    return 0
  })
  return out
}

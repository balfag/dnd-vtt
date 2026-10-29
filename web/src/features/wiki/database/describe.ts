import type { TFunction } from 'i18next'
import type { Filter, Property, SortKey } from '../../../shared/api/wiki'
import { fieldOps, propertyOps, type ValueKind } from './filter'
import { optionById } from './model'

/** opLabel is the word of an operator; numbers use the signs. */
export function opLabel(t: TFunction, prop: Property | undefined, op: string): string {
  return prop?.type === 'number' ? t(`wiki.db.opNumber.${op}`) : t(`wiki.db.op.${op}`)
}

/** opsOf lists the operators allowed for a property, or for a built-in field when prop is undefined. */
export function opsOf(
  prop: Property | undefined,
  field: string | undefined,
): [string, ValueKind][] {
  const table = prop !== undefined ? propertyOps[prop.type] : (fieldOps[field ?? ''] ?? {})
  return Object.entries(table)
}

export function valueKindOf(
  prop: Property | undefined,
  field: string | undefined,
  op: string,
): ValueKind {
  return opsOf(prop, field).find(([name]) => name === op)?.[1] ?? 'none'
}

/** isComplete: the filter has the value its operator needs, as the server wants it. */
export function isComplete(f: Filter, props: Property[]): boolean {
  const prop = props.find((p) => p.id === f.propertyId)
  if (f.propertyId !== undefined && prop === undefined) return false
  const kind = valueKindOf(prop, f.field, f.op)
  switch (kind) {
    case 'none':
      return f.value === undefined
    case 'text':
      return typeof f.value === 'string' && f.value.length >= 1 && f.value.length <= 200
    case 'number':
      return typeof f.value === 'number' && Number.isFinite(f.value)
    case 'option':
      return (
        typeof f.value === 'string' && prop !== undefined && optionById(prop, f.value) !== undefined
      )
    case 'date':
      return typeof f.value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(f.value)
    case 'tag':
      return typeof f.value === 'string' && f.value !== ''
  }
}

/** fieldLabel is the name of the thing a filter or a sort looks at. */
export function fieldLabel(
  t: TFunction,
  props: Property[],
  key: { propertyId?: string; field?: string },
): string {
  if (key.propertyId !== undefined) return props.find((p) => p.id === key.propertyId)?.name ?? ''
  return t(`wiki.db.field.${key.field ?? 'title'}`)
}

/** filterText is the line of an active condition: "Status is not dead". */
export function filterText(
  t: TFunction,
  f: Filter,
  props: Property[],
  tags: ReadonlyMap<string, string>,
): string {
  const prop = props.find((p) => p.id === f.propertyId)
  const parts = [fieldLabel(t, props, f), opLabel(t, prop, f.op)]
  if (f.value !== undefined) {
    if (prop !== undefined && (prop.type === 'select' || prop.type === 'multi_select')) {
      parts.push(optionById(prop, f.value)?.name ?? '')
    } else if (f.field === 'tags' && typeof f.value === 'string') {
      parts.push(tags.get(f.value) ?? '')
    } else {
      parts.push(String(f.value))
    }
  }
  return parts.join(' ')
}

export function sortText(t: TFunction, s: SortKey, props: Property[]): string {
  return `${fieldLabel(t, props, s)} ${s.dir === 'asc' ? '↑' : '↓'}`
}

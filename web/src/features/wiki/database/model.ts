import {
  optionColors,
  type GameLinkValue,
  type OptionColor,
  type Property,
  type SelectOption,
} from '../../../shared/api/wiki'

/** colorValue is the palette of the variants of a select (docs/design/wiki-screen.md, "Цвет"). */
export const colorValue: Record<OptionColor, string> = {
  gray: '#7D8B96',
  brown: '#9A7458',
  orange: '#C27A43',
  yellow: '#BFA85A',
  green: '#6E9F6A',
  blue: '#5E8DB8',
  purple: '#8E78B5',
  red: '#C25468',
}

/** nextColor is the color of a new variant: the next one in the palette, round and round. */
export function nextColor(options: readonly SelectOption[]): OptionColor {
  return optionColors[options.length % optionColors.length] ?? 'gray'
}

export function optionsOf(prop: Property): SelectOption[] {
  return prop.options.options ?? []
}

/** optionById finds the variant of a select by the id stored in a value. */
export function optionById(prop: Property, id: unknown): SelectOption | undefined {
  return optionsOf(prop).find((o) => o.id === id)
}

/** idsOf reads a value that is a list of strings. */
export function idsOf(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}

export function gameLinksOf(v: unknown): GameLinkValue[] {
  if (!Array.isArray(v)) return []
  const out: GameLinkValue[] = []
  for (const x of v as unknown[]) {
    if (
      typeof x === 'object' &&
      x !== null &&
      'kind' in x &&
      'id' in x &&
      (x.kind === 'scene' || x.kind === 'token') &&
      typeof x.id === 'string'
    ) {
      out.push({ kind: x.kind, id: x.id })
    }
  }
  return out
}

/** limits are the sizes of the values the server accepts. */
export const limits = { text: 2000, url: 2000, multiSelect: 20, relation: 50, gameLink: 20 }

/** linkLimit is how many items a value of a list type may hold. */
export function listLimit(prop: Property): number {
  switch (prop.type) {
    case 'multi_select':
      return limits.multiSelect
    case 'relation':
      return limits.relation
    default:
      return limits.gameLink
  }
}

/** isHttpUrl: the value of a url property must start with http:// or https://. */
export function isHttpUrl(s: string): boolean {
  return /^https?:\/\/\S+$/i.test(s)
}

/**
 * parseNumber reads the text of a number field: null for an empty one, undefined for text that is
 * not a finite number, and, for the integer format, for a fraction.
 */
export function parseNumber(text: string, integer: boolean): number | null | undefined {
  const s = text.trim().replace(',', '.')
  if (s === '') return null
  const n = Number(s)
  if (!Number.isFinite(n)) return undefined
  if (integer && !Number.isInteger(n)) return undefined
  return n
}

/** gameLabels remembers the names of scenes and tokens seen so far: a value holds only their ids. */
const gameLabels = new Map<string, string>()

export function rememberGameLabel(kind: string, id: string, label: string): void {
  gameLabels.set(`${kind}:${id}`, label)
}

export function gameLabel(kind: string, id: string): string | undefined {
  return gameLabels.get(`${kind}:${id}`)
}

/** sameValue compares two property values as the server would store them. */
export function sameValue(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
}

/** withKey returns obj with key set to value, or without the key when value is null or undefined. */
export function withKey<T>(
  obj: Readonly<Record<string, T>>,
  key: string,
  value: T | null | undefined,
): Record<string, T> {
  const rest = Object.fromEntries(Object.entries(obj).filter(([k]) => k !== key))
  return value === null || value === undefined ? rest : { ...rest, [key]: value }
}

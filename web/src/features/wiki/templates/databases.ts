import type { TFunction } from 'i18next'
import type { OptionColor, PropertyOptions, PropertyType, ViewKind } from '../../../shared/api/wiki'

type OptionSpec = readonly [key: string, color: OptionColor]

interface PropertySpec {
  key: string
  type: PropertyType
  /** options are the variants of a select, in order. */
  options?: readonly OptionSpec[]
  format?: 'integer' | 'decimal'
}

/** DatabaseTemplate is a database defined in the client; its names come from i18n when the database is made. */
export interface DatabaseTemplate {
  id: 'npc' | 'locations' | 'quests' | 'sessions' | 'items'
  icon: string
  properties: readonly PropertySpec[]
}

/** NewDatabase is the `database` field of a request that makes a database. */
export interface NewDatabase {
  properties: { name: string; type: PropertyType; options: PropertyOptions }[]
  views: { name: string; kind: ViewKind }[]
}

// The properties of docs/systems/gm-wiki.md, "Шаблоны баз". The quest has no relation.
export const databaseTemplates: readonly DatabaseTemplate[] = [
  {
    id: 'npc',
    icon: '🧑',
    properties: [
      {
        key: 'status',
        type: 'select',
        options: [
          ['alive', 'green'],
          ['dead', 'red'],
          ['unknown', 'gray'],
        ],
      },
      {
        key: 'attitude',
        type: 'select',
        options: [
          ['ally', 'green'],
          ['neutral', 'gray'],
          ['enemy', 'red'],
        ],
      },
      { key: 'where', type: 'text' },
      { key: 'token', type: 'game_link' },
    ],
  },
  {
    id: 'locations',
    icon: '🏠',
    properties: [
      {
        key: 'kind',
        type: 'select',
        options: [
          ['city', 'blue'],
          ['dungeon', 'purple'],
          ['wilds', 'green'],
          ['building', 'orange'],
        ],
      },
      { key: 'region', type: 'text' },
      { key: 'scene', type: 'game_link' },
      { key: 'visited', type: 'checkbox' },
    ],
  },
  {
    id: 'quests',
    icon: '🗡',
    properties: [
      {
        key: 'status',
        type: 'select',
        options: [
          ['new', 'gray'],
          ['active', 'blue'],
          ['done', 'green'],
          ['failed', 'red'],
        ],
      },
      { key: 'giver', type: 'text' },
      { key: 'reward', type: 'text' },
    ],
  },
  {
    id: 'sessions',
    icon: '📓',
    properties: [
      { key: 'number', type: 'number', format: 'integer' },
      { key: 'date', type: 'date' },
      { key: 'summary', type: 'text' },
    ],
  },
  {
    id: 'items',
    icon: '🎒',
    properties: [
      {
        key: 'kind',
        type: 'select',
        options: [
          ['weapon', 'red'],
          ['armor', 'blue'],
          ['potion', 'green'],
          ['wondrous', 'purple'],
          ['other', 'gray'],
        ],
      },
      {
        key: 'rarity',
        type: 'select',
        options: [
          ['common', 'gray'],
          ['uncommon', 'green'],
          ['rare', 'blue'],
          ['veryRare', 'purple'],
          ['legendary', 'orange'],
          ['artifact', 'red'],
        ],
      },
      { key: 'owner', type: 'text' },
      { key: 'price', type: 'number', format: 'decimal' },
    ],
  },
]

export function databaseTemplateTitle(t: TFunction, tpl: DatabaseTemplate): string {
  return t(`wiki.db.templates.${tpl.id}.title`)
}

/** newViews are the two views every new database starts with: a table and cards. */
export function newViews(t: TFunction): NewDatabase['views'] {
  return [
    { name: t('wiki.db.view.table'), kind: 'table' },
    { name: t('wiki.db.view.cards'), kind: 'cards' },
  ]
}

/** buildDatabase makes the properties and views of a new database in the language of t; no template gives an empty one. */
export function buildDatabase(t: TFunction, tpl?: DatabaseTemplate): NewDatabase {
  const properties = (tpl?.properties ?? []).map((p) => {
    const base = `wiki.db.templates.${tpl?.id ?? ''}`
    let options: PropertyOptions = {}
    if (p.options !== undefined) {
      options = {
        options: p.options.map(([key, color]) => ({
          name: t(`${base}.opts.${p.key}.${key}`),
          color,
        })),
      }
    } else if (p.format !== undefined) {
      options = { format: p.format }
    }
    return { name: t(`${base}.props.${p.key}`), type: p.type, options }
  })
  return { properties, views: newViews(t) }
}

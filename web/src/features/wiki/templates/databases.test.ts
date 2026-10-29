import { describe, expect, it } from 'vitest'
import i18n from '../../../i18n'
import { optionColors, propertyTypes } from '../../../shared/api/wiki'
import { buildDatabase, databaseTemplates, databaseTemplateTitle } from './databases'

describe.each(['ru', 'en'])('database templates in %s', (lng) => {
  const t = i18n.getFixedT(lng)

  it('are the five of the spec', () => {
    expect(databaseTemplates.map((d) => d.id)).toEqual([
      'npc',
      'locations',
      'quests',
      'sessions',
      'items',
    ])
  })

  it.each(databaseTemplates.map((d) => [d.id, d] as const))(
    '%s is a request the server accepts',
    (_id, tpl) => {
      const db = buildDatabase(t, tpl)
      const names = db.properties.map((p) => p.name)
      expect(databaseTemplateTitle(t, tpl)).not.toBe('')
      expect(db.properties.length).toBeGreaterThan(0)
      expect(db.properties.length).toBeLessThanOrEqual(30)
      expect(new Set(names).size).toBe(names.length)
      for (const p of db.properties) {
        expect(p.name.length).toBeGreaterThanOrEqual(1)
        expect(p.name.length).toBeLessThanOrEqual(100)
        expect(propertyTypes).toContain(p.type)
        if (p.type === 'select' || p.type === 'multi_select') {
          const options = p.options.options ?? []
          expect(options.length).toBeGreaterThan(0)
          expect(options.length).toBeLessThanOrEqual(100)
          expect(new Set(options.map((o) => o.name)).size).toBe(options.length)
          for (const o of options) {
            expect(optionColors).toContain(o.color)
            expect(o.name.length).toBeGreaterThanOrEqual(1)
            expect(o.name.length).toBeLessThanOrEqual(100)
            expect(o).not.toHaveProperty('id')
          }
        }
      }
      // A name that still looks like a key means the translation is missing.
      expect(JSON.stringify(db)).not.toContain('wiki.db')
    },
  )

  it('give a table and cards, in the language', () => {
    const { views } = buildDatabase(t, databaseTemplates[0])
    expect(views.map((v) => v.kind)).toEqual(['table', 'cards'])
    expect(views.every((v) => v.name !== '' && !v.name.includes('wiki.db'))).toBe(true)
  })

  it('make an empty database without a template', () => {
    const db = buildDatabase(t)
    expect(db.properties).toEqual([])
    expect(db.views).toHaveLength(2)
  })

  it('have no relation: the quest of the spec has none', () => {
    for (const tpl of databaseTemplates) {
      expect(buildDatabase(t, tpl).properties.some((p) => p.type === 'relation')).toBe(false)
    }
  })
})

describe('the NPC template', () => {
  it('is the table of the spec, in Russian', () => {
    const t = i18n.getFixedT('ru')
    const npc = databaseTemplates.find((d) => d.id === 'npc')
    const db = buildDatabase(t, npc)
    expect(db.properties.map((p) => [p.name, p.type])).toEqual([
      ['Статус', 'select'],
      ['Отношение', 'select'],
      ['Где встречается', 'text'],
      ['Фишка', 'game_link'],
    ])
    expect(db.properties[0]?.options.options?.map((o) => o.name)).toEqual([
      'жив',
      'мёртв',
      'неизвестно',
    ])
  })
})

describe('icons', () => {
  it('are one character each', () => {
    for (const d of databaseTemplates) {
      expect([...new Intl.Segmenter().segment(d.icon)]).toHaveLength(1)
    }
  })
})

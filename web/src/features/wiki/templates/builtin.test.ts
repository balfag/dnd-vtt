import { getSchema } from '@tiptap/core'
import { Node as PMNode } from '@tiptap/pm/model'
import { describe, expect, it } from 'vitest'
import i18n from '../../../i18n'
import { createExtensions } from '../editor/extensions'
import { HandlersRef } from '../editor/handlers'
import { builtinTemplates } from './builtin'

const schema = getSchema(createExtensions({ handlers: new HandlersRef() }))

describe.each(['ru', 'en'])('builtin templates in %s', (lng) => {
  const t = i18n.getFixedT(lng)

  it.each(builtinTemplates.map((b) => [b.id, b] as const))('%s fits the editor schema', (_id, b) => {
    const node = PMNode.fromJSON(schema, b.content(t))
    node.check()
    expect(b.title(t)).not.toBe('')
    expect(b.title(t)).not.toContain('wiki.templates')
    expect(JSON.stringify(b.content(t))).not.toContain('wiki.templates')
  })

  it('has headings of level 2 and, for the quest, a checklist of three items', () => {
    const quest = builtinTemplates.find((b) => b.id === 'quest')
    const json = JSON.stringify(quest?.content(t))
    expect(json).toContain('"taskList"')
    expect(json.match(/"taskItem"/g)).toHaveLength(3)
  })
})

describe('icons', () => {
  it('are one character each', () => {
    for (const b of builtinTemplates) {
      expect([...new Intl.Segmenter().segment(b.icon)]).toHaveLength(1)
    }
  })
})
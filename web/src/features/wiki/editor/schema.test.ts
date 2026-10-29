import { getSchema } from '@tiptap/core'
import { Node as PMNode, type Schema } from '@tiptap/pm/model'
import { EditorState } from '@tiptap/pm/state'
import { describe, expect, it } from 'vitest'
import goSchema from '../../../../../internal/wiki/doc/schema.go?raw'
import golden from '../../../../../internal/wiki/doc/testdata/full.golden.json'
import { createExtensions } from './extensions'
import { HandlersRef } from './handlers'
import { fixAttrsPlugin } from './normalize'
import { rollFromInput } from './rules'

const schema = getSchema(createExtensions({ handlers: new HandlersRef() }))

// The attributes of schema v1 (docs/architecture/protocol.md, "Содержимое страницы"). The server
// drops every other attribute, so the editor's own ones (TipTap adds some) do not count.
const v1Attrs: Record<string, string[]> = {
  heading: ['level'],
  orderedList: ['start'],
  taskItem: ['checked'],
  callout: ['emoji'],
  codeBlock: ['language'],
  tableCell: ['colspan', 'rowspan', 'colwidth'],
  tableHeader: ['colspan', 'rowspan', 'colwidth'],
  image: ['assetId', 'alt'],
  details: ['open'],
  mention: ['kind', 'id', 'label'],
  inlineRoll: ['formula'],
  link: ['href'],
}

interface Json {
  type: string
  attrs?: Record<string, unknown>
  marks?: Json[]
  content?: Json[]
  text?: string
}

function only(
  attrs: Record<string, unknown> | undefined,
  type: string,
): Record<string, unknown> | undefined {
  const keep = v1Attrs[type] ?? []
  const kept = Object.entries(attrs ?? {}).filter(([k]) => keep.includes(k))
  return kept.length > 0 ? Object.fromEntries(kept) : undefined
}

// stripToV1 removes the attributes that are not in schema v1 and the empty attrs/content keys.
function stripToV1(node: Json): Json {
  const out: Json = { type: node.type }
  const attrs = only(node.attrs, node.type)
  if (attrs) out.attrs = attrs
  // The editor keeps marks in the order of its schema, the server in the order it was sent.
  if (node.marks) out.marks = node.marks.map(stripToV1).sort((a, b) => a.type.localeCompare(b.type))
  if (node.content && node.content.length > 0) out.content = node.content.map(stripToV1)
  if (node.text !== undefined) out.text = node.text
  return out
}

// goServerNames reads the node and mark names the server accepts from internal/wiki/doc/schema.go.
function goServerNames(): { nodes: string[]; marks: string[] } {
  const src = goSchema
  const keys = (start: string): string[] => {
    const from = src.indexOf(start)
    const to = src.indexOf('\n}\n', from)
    return [...src.slice(from, to).matchAll(/^\t"([A-Za-z]+)":/gm)].map((m) => m[1] ?? '')
  }
  return { nodes: keys('var nodeSpecs'), marks: keys('var markSpecs') }
}

// A DOM element is enough for the parse rules of the schema: they only read attributes.
function fakeElement(attrs: Record<string, string>): HTMLElement {
  return {
    getAttribute: (name: string) => attrs[name] ?? null,
    hasAttribute: (name: string) => name in attrs,
    children: [],
    parentElement: null,
    closest: () => null,
    textContent: '',
    style: {},
  } as unknown as HTMLElement
}

function parseAttrs(
  s: Schema,
  kind: 'node' | 'mark',
  name: string,
  tag: string,
  attrs: Record<string, string>,
) {
  const spec = (kind === 'node' ? s.nodes[name] : s.marks[name])?.spec
  const rule = spec?.parseDOM?.find((r) => r.tag === tag)
  if (!rule?.getAttrs) throw new Error(`no parse rule ${tag} with getAttrs for ${name}`)
  // ProseMirror types the argument of getAttrs as HTMLElement & string.
  return rule.getAttrs(fakeElement(attrs) as HTMLElement & string)
}

describe('schema v1', () => {
  it('loads the normalized server document and gives it back unchanged', () => {
    const doc = PMNode.fromJSON(schema, golden)
    expect(stripToV1(doc.toJSON() as Json)).toEqual(stripToV1(golden as Json))
  })

  it('has valid content (the golden document, without the marks that exclude each other)', () => {
    // The server allows "code" together with other marks; in the editor "code" excludes them.
    const prune = (n: Json): Json => ({
      ...n,
      ...(n.marks && n.marks.length > 1 ? { marks: n.marks.filter((m) => m.type !== 'code') } : {}),
      ...(n.content ? { content: n.content.map(prune) } : {}),
    })
    expect(() => {
      PMNode.fromJSON(schema, prune(golden)).check()
    }).not.toThrow()
  })

  it('has exactly the nodes and marks the server knows', () => {
    const server = goServerNames()
    expect(Object.keys(schema.nodes).sort()).toEqual([...server.nodes].sort())
    expect(Object.keys(schema.marks).sort()).toEqual([...server.marks].sort())
  })

  it('gives every attribute that the server requires', () => {
    const required: Record<string, string[]> = {
      image: ['assetId'],
      mention: ['kind', 'id'],
      inlineRoll: ['formula'],
    }
    for (const [type, names] of Object.entries(required)) {
      const attrs = schema.nodes[type]?.spec.attrs ?? {}
      for (const name of names) expect(attrs, `${type}.${name}`).toHaveProperty(name)
    }
    expect(schema.marks.link?.spec.attrs).toHaveProperty('href')
  })

  it('has headings of levels 1-3 only', () => {
    const tags = schema.nodes.heading?.spec.parseDOM?.map((r) => r.tag)
    expect(tags).toEqual(['h1', 'h2', 'h3'])
  })

  it('never merges table cells, whatever a pasted table says', () => {
    for (const name of ['tableCell', 'tableHeader']) {
      const tag = name === 'tableCell' ? 'td' : 'th'
      expect(parseAttrs(schema, 'node', name, tag, { colspan: '3', rowspan: '2' })).toMatchObject({
        colspan: 1,
        rowspan: 1,
      })
    }
  })

  it('drops a pasted mention that the server would reject', () => {
    const ok = { 'data-kind': 'scene', 'data-id': '01HZX3K9M2N4P6Q8R0S1T2V3W4' }
    expect(parseAttrs(schema, 'node', 'mention', 'span[data-type="mention"]', ok)).not.toBe(false)
    expect(
      parseAttrs(schema, 'node', 'mention', 'span[data-type="mention"]', {
        ...ok,
        'data-kind': 'x',
      }),
    ).toBe(false)
    expect(
      parseAttrs(schema, 'node', 'mention', 'span[data-type="mention"]', {
        ...ok,
        'data-id': 'abc',
      }),
    ).toBe(false)
  })

  it('drops a pasted roll with a bad formula', () => {
    const tag = 'span[data-type="inline-roll"]'
    expect(parseAttrs(schema, 'node', 'inlineRoll', tag, { 'data-formula': '2d6+3' })).not.toBe(
      false,
    )
    expect(parseAttrs(schema, 'node', 'inlineRoll', tag, { 'data-formula': '2d' })).toBe(false)
  })

  it('takes only images of campaign assets', () => {
    const tags = schema.nodes.image?.spec.parseDOM?.map((r) => r.tag)
    expect(tags).toEqual(['img[data-asset-id]'])
  })

  it('does not make a link of a javascript: address', () => {
    const link = (href: string) => parseAttrs(schema, 'mark', 'link', 'a[href]', { href })
    expect(link('javascript:alert(1)')).toBe(false)
    expect(link('data:text/html,x')).toBe(false)
    expect(link('https://example.com')).not.toBe(false)
    expect(link('mailto:a@b.c')).not.toBe(false)
  })
})

describe('rollFromInput', () => {
  it.each([
    ['[[2d6+3]]', '2d6+3'],
    ['text [[d20]]', 'd20'],
    ['[[ 2d20kh1 + 5 ]]', '2d20kh1 + 5'],
  ])('%s is a roll', (typed, formula) => {
    expect(rollFromInput(typed)).toBe(formula)
  })

  it.each(['[[2d]]', '[[]]', '[[ ]]', '[[abc]]', '[[1d6r<=6]]', '[2d6]', '[[2d6]'])(
    '%s stays text',
    (typed) => {
      expect(rollFromInput(typed)).toBeNull()
    },
  )
})

describe('fixAttrs', () => {
  const apply = (doc: Json) => {
    const state = EditorState.create({
      doc: PMNode.fromJSON(schema, doc),
      plugins: [fixAttrsPlugin],
    })
    // Any edit makes the plugin look at the document: type into the first text.
    let at = 1
    state.doc.descendants((node, pos) => {
      if (node.isText && at === 1) at = pos + 1
      return at === 1
    })
    return state.apply(state.tr.insertText('x', at)).doc.toJSON() as Json
  }

  it('clears a code block language outside [a-z0-9+#-]{1,20}', () => {
    const block = (language: string): Json => ({
      type: 'doc',
      content: [{ type: 'codeBlock', attrs: { language }, content: [{ type: 'text', text: 'a' }] }],
    })
    expect(apply(block('js')).content?.[0]?.attrs?.language).toBe('js')
    expect(apply(block('a'.repeat(21))).content?.[0]?.attrs?.language).toBeNull()
    expect(apply(block('C#')).content?.[0]?.attrs?.language).toBeNull()
  })

  it('brings a column width into 40..2000', () => {
    const table = (colwidth: number[]): Json => ({
      type: 'doc',
      content: [
        {
          type: 'table',
          content: [
            {
              type: 'tableRow',
              content: [
                {
                  type: 'tableCell',
                  attrs: { colwidth },
                  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'a' }] }],
                },
              ],
            },
          ],
        },
      ],
    })
    const width = (doc: Json) => doc.content?.[0]?.content?.[0]?.content?.[0]?.attrs?.colwidth
    expect(width(apply(table([25])))).toEqual([40])
    expect(width(apply(table([5000])))).toEqual([2000])
    expect(width(apply(table([120])))).toEqual([120])
    expect(width(apply(table([100, 200])))).toEqual([100])
    expect(width(apply(table([Number.NaN])))).toBeNull()
  })
})

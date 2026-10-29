import { getSchema } from '@tiptap/core'
import { Node as PMNode } from '@tiptap/pm/model'
import { describe, expect, it } from 'vitest'
import { createExtensions } from '../wiki/editor/extensions'
import { HandlersRef } from '../wiki/editor/handlers'
import { MAX_HANDOUT_BYTES, toHandout } from './toHandout'

const schema = getSchema(createExtensions({ handlers: new HandlersRef() }))
const ID = '01HZZZZZZZZZZZZZZZZZZZZZZZ'

const mention = (label: string) => ({ type: 'mention', attrs: { kind: 'scene', id: ID, label } })
const paragraph = (...content: unknown[]) => ({ type: 'paragraph', content })
const doc = (...content: unknown[]) => ({ type: 'doc', content })

describe('toHandout', () => {
  it('turns a mention into the text of its current name', () => {
    const { doc: out } = toHandout(
      doc(paragraph({ type: 'text', text: 'Go to ' }, mention('Old name'))),
      (kind, id) => (kind === 'scene' && id === ID ? 'New name' : null),
    )
    expect(out).toStrictEqual(
      doc(paragraph({ type: 'text', text: 'Go to ' }, { type: 'text', text: 'New name' })),
    )
  })

  it('keeps the stored label of a mention whose name is not known', () => {
    const { doc: out } = toHandout(doc(paragraph(mention('Stored'))), () => null)
    expect(out).toStrictEqual(doc(paragraph({ type: 'text', text: 'Stored' })))
  })

  it('turns an inline roll into the text of its formula', () => {
    const { doc: out } = toHandout(
      doc(paragraph({ type: 'inlineRoll', attrs: { formula: '2d6+3' } })),
      () => null,
    )
    expect(out).toStrictEqual(doc(paragraph({ type: 'text', text: '2d6+3' })))
  })

  it('drops a mention with no name at all and leaves an empty paragraph alone', () => {
    const { doc: out } = toHandout(doc(paragraph(mention(''))), () => null)
    expect(out).toStrictEqual(doc({ type: 'paragraph' }))
  })

  it('works inside lists and keeps marks and other nodes', () => {
    const list = {
      type: 'bulletList',
      content: [
        {
          type: 'listItem',
          content: [
            paragraph({ type: 'text', text: 'a', marks: [{ type: 'bold' }] }, mention('X')),
          ],
        },
      ],
    }
    const image = { type: 'image', attrs: { assetId: ID, alt: '' } }
    const { doc: out } = toHandout(doc(list, image), () => 'Y')
    expect(out).toStrictEqual(
      doc(
        {
          type: 'bulletList',
          content: [
            {
              type: 'listItem',
              content: [
                paragraph(
                  { type: 'text', text: 'a', marks: [{ type: 'bold' }] },
                  { type: 'text', text: 'Y' },
                ),
              ],
            },
          ],
        },
        image,
      ),
    )
  })

  it('gives a document the editor schema accepts', () => {
    const { doc: out } = toHandout(
      doc(
        paragraph(
          { type: 'text', text: 'Roll ' },
          { type: 'inlineRoll', attrs: { formula: '1d20' } },
          { type: 'text', text: ' near ' },
          mention('Hall'),
        ),
      ),
      () => 'Great hall',
    )
    expect(() => {
      PMNode.fromJSON(schema, out).check()
    }).not.toThrow()
    expect(JSON.stringify(out)).not.toContain('mention')
    expect(JSON.stringify(out)).not.toContain('inlineRoll')
  })

  it('gives an empty paragraph for an empty fragment', () => {
    expect(toHandout(doc(), () => null).doc).toStrictEqual(doc({ type: 'paragraph' }))
    expect(toHandout(null, () => null).doc).toStrictEqual(doc({ type: 'paragraph' }))
  })

  it('counts the size in UTF-8 bytes', () => {
    const latin = toHandout(doc(paragraph({ type: 'text', text: 'abcd' })), () => null)
    const cyrillic = toHandout(doc(paragraph({ type: 'text', text: 'абвг' })), () => null)
    // Each Cyrillic letter is two bytes.
    expect(cyrillic.bytes - latin.bytes).toBe(4)
    expect(latin.bytes).toBe(JSON.stringify(latin.doc).length)
  })

  it('limits a handout to 48 KiB', () => {
    expect(MAX_HANDOUT_BYTES).toBe(49152)
  })
})

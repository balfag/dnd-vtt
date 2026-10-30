// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { docOf, isElement, isHTMLElement, isNode } from './dom'

const foreignElement = { nodeType: 1, nodeName: 'DIV', contains: () => true, focus: () => {} }
const foreignSvg = { nodeType: 1, nodeName: 'svg', contains: () => true }
const foreignText = { nodeType: 3, nodeName: '#text', contains: () => false }

describe('isNode', () => {
  it('accepts a node of another window', () => {
    expect(isNode(foreignElement)).toBe(true)
    expect(isNode(foreignText)).toBe(true)
  })
  it('rejects everything else', () => {
    expect(isNode(null)).toBe(false)
    expect(isNode({})).toBe(false)
    expect(isNode('div')).toBe(false)
    expect(isNode({ nodeType: 'x' })).toBe(false)
  })
})

describe('isElement', () => {
  it('accepts elements only', () => {
    expect(isElement(foreignElement)).toBe(true)
    expect(isElement(foreignSvg)).toBe(true)
    expect(isElement(foreignText)).toBe(false)
    expect(isElement(null)).toBe(false)
  })
})

describe('isHTMLElement', () => {
  it('needs focus()', () => {
    expect(isHTMLElement(foreignElement)).toBe(true)
    expect(isHTMLElement(foreignSvg)).toBe(false)
    expect(isHTMLElement({})).toBe(false)
  })
})

describe('docOf', () => {
  it('returns the owner document of the element', () => {
    const doc = { nodeType: 9 }
    expect(docOf({ ownerDocument: doc } as unknown as Node)).toBe(doc)
  })
  it('returns the document itself for a document node', () => {
    const doc = { nodeType: 9, ownerDocument: null }
    expect(docOf(doc as unknown as Node)).toBe(doc)
  })
})

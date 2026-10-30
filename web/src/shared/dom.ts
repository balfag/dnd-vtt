// Window-independent DOM checks. A detached panel lives in a window of its own with its own Node,
// Element and HTMLElement constructors, so `instanceof` against the main window's ones fails there.

const ELEMENT_NODE = 1

/** isNode reports whether v is a DOM node of any window (instanceof fails across windows). */
export function isNode(v: unknown): v is Node {
  if (typeof v !== 'object' || v === null) return false
  const n = v as { nodeType?: unknown; contains?: unknown; nodeName?: unknown }
  return (
    typeof n.nodeType === 'number' &&
    (typeof n.contains === 'function' || typeof n.nodeName === 'string')
  )
}

/** isElement — isNode and nodeType === Node.ELEMENT_NODE (1). */
export function isElement(v: unknown): v is Element {
  return isNode(v) && v.nodeType === ELEMENT_NODE
}

/** isHTMLElement — isElement and the element has focus() (an HTML element of any window). */
export function isHTMLElement(v: unknown): v is HTMLElement {
  return isElement(v) && typeof (v as { focus?: unknown }).focus === 'function'
}

/** docOf returns the document an element lives in (its ownerDocument), or the global document for null. */
export function docOf(el: Node | null | undefined): Document {
  if (el === null || el === undefined) return document
  return el.ownerDocument ?? (el as Document)
}

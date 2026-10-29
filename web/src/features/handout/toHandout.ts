/** MAX_HANDOUT_BYTES is the biggest handout the server takes: the command has to fit one WebSocket frame. */
export const MAX_HANDOUT_BYTES = 48 * 1024

interface JsonNode {
  type: string
  attrs?: Record<string, unknown>
  marks?: unknown[]
  content?: unknown[]
  text?: string
}

function isNode(v: unknown): v is JsonNode {
  return typeof v === 'object' && v !== null && typeof (v as { type?: unknown }).type === 'string'
}

function textNode(text: string): JsonNode[] {
  return text === '' ? [] : [{ type: 'text', text }]
}

function convert(
  node: JsonNode,
  resolveLabel: (kind: string, id: string) => string | null,
): JsonNode[] {
  if (node.type === 'mention') {
    const kind = String(node.attrs?.kind ?? '')
    const id = String(node.attrs?.id ?? '')
    const stored = String(node.attrs?.label ?? '')
    return textNode(resolveLabel(kind, id) ?? stored)
  }
  if (node.type === 'inlineRoll') {
    return textNode(String(node.attrs?.formula ?? ''))
  }
  if (!Array.isArray(node.content)) return [node]
  const content = node.content.filter(isNode).flatMap((child) => convert(child, resolveLabel))
  const rest: JsonNode = { ...node }
  delete rest.content
  return [content.length === 0 ? rest : { ...rest, content }]
}

/**
 * toHandout replaces mentions with text of their current label and inline rolls with their formula,
 * drops nodes the handout schema forbids, and returns the doc and its JSON size in bytes (UTF-8).
 * resolveLabel returns the current name of a mention target, or null when it is not known: the label
 * stored in the mention stays then.
 */
export function toHandout(
  doc: unknown,
  resolveLabel: (kind: string, id: string) => string | null,
): { doc: unknown; bytes: number } {
  const root = isNode(doc) ? doc : { type: 'doc' }
  const content = (Array.isArray(root.content) ? root.content : [])
    .filter(isNode)
    .flatMap((child) => convert(child, resolveLabel))
  const out = { type: 'doc', content: content.length === 0 ? [{ type: 'paragraph' }] : content }
  return { doc: out, bytes: new TextEncoder().encode(JSON.stringify(out)).length }
}

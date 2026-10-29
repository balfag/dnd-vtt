import type { ChatMessage, PlayerInfo } from '../../shared/protocol'

/** HandoutAudience tells who a handout of the chat was shown to, as the viewer should read it. */
export type HandoutAudience =
  { kind: 'everyone' } | { kind: 'me' } | { kind: 'names'; names: string }

/** handoutAudience says whether a handout went to everyone, to the viewer alone among others, or to some players by name. */
export function handoutAudience(
  message: ChatMessage,
  players: PlayerInfo[],
  viewerId: string | null,
): HandoutAudience {
  if (message.visibility !== 'private') return { kind: 'everyone' }
  const to = message.to ?? []
  if (viewerId !== null && to.includes(viewerId)) return { kind: 'me' }
  const names = to
    .map((id) => players.find((p) => p.id === id)?.displayName ?? '')
    .filter((name) => name !== '')
    .join(', ')
  return { kind: 'names', names }
}

/** firstImageAsset returns the asset id of the first image in a handout document, or null. */
export function firstImageAsset(content: unknown): string | null {
  if (typeof content !== 'object' || content === null) return null
  const node = content as { type?: unknown; attrs?: { assetId?: unknown }; content?: unknown }
  if (node.type === 'image' && typeof node.attrs?.assetId === 'string') return node.attrs.assetId
  if (!Array.isArray(node.content)) return null
  for (const child of node.content) {
    const found = firstImageAsset(child)
    if (found !== null) return found
  }
  return null
}

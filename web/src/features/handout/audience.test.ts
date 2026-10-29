import { describe, expect, it } from 'vitest'
import type { ChatMessage, PlayerInfo } from '../../shared/protocol'
import { firstImageAsset, handoutAudience } from './audience'

const player = (id: string, displayName: string): PlayerInfo => ({
  id,
  displayName,
  color: '#aa0000',
  role: 'player',
  online: true,
})
const players = [player('p2', 'Anna'), player('p3', 'Boris'), player('p4', 'Vika')]

function handout(visibility: ChatMessage['visibility'], to?: string[]): ChatMessage {
  return {
    id: 'h1',
    createdAt: 1,
    authorPlayerId: 'p1',
    kind: 'handout',
    visibility,
    ...(to === undefined ? {} : { to }),
    handout: { title: 'Note', content: { type: 'doc' } },
  }
}

describe('handoutAudience', () => {
  it('is everyone for a public handout', () => {
    expect(handoutAudience(handout('public'), players, 'p2')).toEqual({ kind: 'everyone' })
  })

  it('is the viewer for a private handout they are a recipient of', () => {
    expect(handoutAudience(handout('private', ['p2', 'p3']), players, 'p2')).toEqual({ kind: 'me' })
  })

  it('names the recipients to the GM, who is not among them', () => {
    expect(handoutAudience(handout('private', ['p2', 'p3']), players, 'p1')).toEqual({
      kind: 'names',
      names: 'Anna, Boris',
    })
  })

  it('leaves out a recipient that is not known', () => {
    expect(handoutAudience(handout('private', ['p2', 'gone']), players, null)).toEqual({
      kind: 'names',
      names: 'Anna',
    })
  })
})

describe('firstImageAsset', () => {
  it('finds the first image at any depth', () => {
    const doc = {
      type: 'doc',
      content: [
        { type: 'paragraph' },
        {
          type: 'bulletList',
          content: [{ type: 'image', attrs: { assetId: 'a1', alt: '' } }],
        },
        { type: 'image', attrs: { assetId: 'a2', alt: '' } },
      ],
    }
    expect(firstImageAsset(doc)).toBe('a1')
  })

  it('is null without an image', () => {
    expect(firstImageAsset({ type: 'doc', content: [{ type: 'paragraph' }] })).toBeNull()
    expect(firstImageAsset(null)).toBeNull()
  })
})

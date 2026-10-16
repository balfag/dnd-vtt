import { describe, expect, it } from 'vitest'
import type { CampaignPlayer } from '../../shared/api/campaigns'
import type { PlayerInfo } from '../../shared/protocol'
import { hasUnknownParticipant, mergeParticipants } from './participants'

function rest(id: string, patch: Partial<CampaignPlayer> = {}): CampaignPlayer {
  return {
    id,
    accountId: `a-${id}`,
    accountLogin: id,
    displayName: id.toUpperCase(),
    color: '#e53935',
    role: 'player',
    removed: false,
    online: false,
    ...patch,
  }
}

function live(id: string, online: boolean): PlayerInfo {
  return { id, displayName: id, color: '#e53935', role: 'player', online }
}

describe('mergeParticipants', () => {
  it('drops the removed participants', () => {
    const merged = mergeParticipants([rest('p1'), rest('p2', { removed: true }), rest('p3')], null)
    expect(merged.map((p) => p.id)).toEqual(['p1', 'p3'])
  })

  it('takes the online flag from live when it has the participant', () => {
    const merged = mergeParticipants(
      [rest('p1', { online: false }), rest('p2', { online: true })],
      [live('p1', true), live('p2', false)],
    )
    expect(merged.map((p) => p.online)).toEqual([true, false])
  })

  it('keeps the REST flag without live, or when live lacks the participant', () => {
    expect(mergeParticipants([rest('p1', { online: true })], null)[0]?.online).toBe(true)
    expect(mergeParticipants([rest('p1', { online: true })], [live('p9', false)])[0]?.online).toBe(
      true,
    )
  })

  it('keeps the order of the REST list, not of live', () => {
    const merged = mergeParticipants(
      [rest('p2'), rest('p1'), rest('p3')],
      [live('p3', true), live('p1', true)],
    )
    expect(merged.map((p) => p.id)).toEqual(['p2', 'p1', 'p3'])
  })

  it('does not bring back a removed participant that live still lists', () => {
    const merged = mergeParticipants([rest('p1', { removed: true })], [live('p1', true)])
    expect(merged).toEqual([])
  })
})

describe('hasUnknownParticipant', () => {
  it('finds a participant that REST does not have', () => {
    expect(hasUnknownParticipant([rest('p1')], [live('p1', true), live('p2', true)])).toBe(true)
    expect(hasUnknownParticipant([rest('p1'), rest('p2')], [live('p2', false)])).toBe(false)
    expect(hasUnknownParticipant([], [])).toBe(false)
  })
})

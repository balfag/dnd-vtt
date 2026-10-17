import { describe, expect, it } from 'vitest'
import type { Combat, Token } from '../../shared/protocol'
import { turnChange } from './turn'

function token(id: string, owners: string[]): Token {
  return {
    id,
    sceneId: 's1',
    layer: 'tokens',
    x: 0,
    y: 0,
    w: 1,
    h: 1,
    rotation: 0,
    z: 0,
    name: id,
    nameVisibility: 'all',
    image: null,
    hidden: false,
    locked: false,
    owners,
    bars: [],
    conditions: [],
  }
}

const tokens = [token('t-ann', ['p2']), token('t-orc', [])]
const me = { isGM: false, playerId: 'p2' }

function fight(turn: string | null, round = 1, sceneId = 's1'): Combat {
  return {
    sceneId,
    round,
    turnCombatantId: turn,
    combatants: [
      { id: 'c-ann', tokenId: 't-ann', name: 'Ann', initiative: 15, hidden: false },
      { id: 'c-orc', tokenId: 't-orc', name: 'Orc', initiative: 9, hidden: false },
    ],
  }
}

describe('turnChange', () => {
  it('announces the turn of someone else', () => {
    expect(turnChange(fight('c-ann'), fight('c-orc'), tokens, me)).toEqual({
      name: 'Orc',
      mine: false,
    })
  })

  it('knows the turn is the viewer’s', () => {
    expect(turnChange(fight('c-orc'), fight('c-ann'), tokens, me)).toEqual({
      name: 'Ann',
      mine: true,
    })
  })

  it('announces the first turn of a fight that was already there', () => {
    expect(turnChange(fight(null), fight('c-ann'), tokens, me)?.mine).toBe(true)
  })

  it('announces a new round that gives the turn to the same combatant', () => {
    expect(turnChange(fight('c-ann', 1), fight('c-ann', 2), tokens, me)?.mine).toBe(true)
  })

  it('is silent when the turn did not move', () => {
    expect(turnChange(fight('c-ann'), fight('c-ann'), tokens, me)).toBeNull()
  })

  it('is silent for a fight that has just appeared, ended or moved to another scene', () => {
    expect(turnChange(null, fight('c-ann'), tokens, me)).toBeNull()
    expect(turnChange(fight('c-ann'), null, tokens, me)).toBeNull()
    expect(turnChange(fight('c-orc', 1, 's0'), fight('c-ann'), tokens, me)).toBeNull()
  })

  it('is silent when the turn is of a combatant the viewer does not see', () => {
    expect(turnChange(fight('c-ann'), fight(null), tokens, me)).toBeNull()
  })

  it('does not treat a viewer without a player id as an owner', () => {
    expect(
      turnChange(fight('c-orc'), fight('c-ann'), tokens, { isGM: true, playerId: null }),
    ).toEqual({ name: 'Ann', mine: false })
  })
})

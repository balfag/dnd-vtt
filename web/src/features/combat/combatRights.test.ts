import { describe, expect, it } from 'vitest'
import type { Combat, Combatant, Token } from '../../shared/protocol'
import { canEndTurn, canSetInitiative, type Me } from './combatRights'

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

function combatant(id: string, tokenId: string): Combatant {
  return { id, tokenId, name: id, initiative: null, hidden: false }
}

const tokens = [token('t-ann', ['p2']), token('t-orc', [])]
const ann = combatant('c-ann', 't-ann')
const orc = combatant('c-orc', 't-orc')
const gone = combatant('c-gone', 't-gone')

const gm: Me = { isGM: true, playerId: 'p1' }
const owner: Me = { isGM: false, playerId: 'p2' }
const other: Me = { isGM: false, playerId: 'p3' }
const stranger: Me = { isGM: false, playerId: null }

function fight(turn: string | null): Combat {
  return { sceneId: 's1', round: 1, turnCombatantId: turn, combatants: [ann, orc, gone] }
}

describe('canSetInitiative', () => {
  it.each([
    ['the GM, own token', gm, ann, true],
    ['the GM, token without owners', gm, orc, true],
    ['the GM, token that is not on the scene', gm, gone, true],
    ['the owner of the token', owner, ann, true],
    ['the owner, token of someone else', owner, orc, false],
    ['another player, token of the owner', other, ann, false],
    ['another player, token without owners', other, orc, false],
    ['a viewer without a player id', stranger, ann, false],
    ['a player, token that is not on the scene', owner, gone, false],
  ])('%s', (_name, me, c, want) => {
    expect(canSetInitiative(c, tokens, me)).toBe(want)
  })
})

describe('canEndTurn', () => {
  it.each([
    ['the GM, turn of a player', gm, 'c-ann', true],
    ['the GM, turn of a token without owners', gm, 'c-orc', true],
    ['the GM, before the first turn', gm, null, true],
    ['the owner, own turn', owner, 'c-ann', true],
    ['the owner, turn of someone else', owner, 'c-orc', false],
    ['another player, turn of the owner', other, 'c-ann', false],
    ['another player, token without owners', other, 'c-orc', false],
    ['the owner, before the first turn', owner, null, false],
    ['the owner, turn of an unknown combatant', owner, 'c-none', false],
    ['the owner, token that is not on the scene', owner, 'c-gone', false],
    ['a viewer without a player id', stranger, 'c-ann', false],
  ])('%s', (_name, me, turn, want) => {
    expect(canEndTurn(fight(turn), tokens, me)).toBe(want)
  })
})

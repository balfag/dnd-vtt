import { describe, expect, it } from 'vitest'
import type { PlayerInfo, Token } from '../../../shared/protocol'
import { tokenStyle } from './tokenStyle'

const players: PlayerInfo[] = [
  { id: 'p1', displayName: 'Ann', color: '#c0392b', role: 'player', online: true },
  { id: 'p2', displayName: 'Bob', color: '#2980b9', role: 'player', online: true },
]

function token(over: Partial<Token> = {}): Token {
  return {
    id: 't1',
    sceneId: 's1',
    layer: 'tokens',
    x: 0,
    y: 0,
    w: 1,
    h: 1,
    rotation: 0,
    z: 0,
    name: 'goblin',
    nameVisibility: 'all',
    image: null,
    hidden: false,
    locked: false,
    owners: [],
    bars: [],
    conditions: [],
    ...over,
  }
}

describe('tokenStyle', () => {
  it('draws a token without owners with the cool-red fill and a thin chalk ring', () => {
    const s = tokenStyle(token(), players, false)
    expect(s).toMatchObject({
      fill: '#4a2a2f',
      stroke: '#ECE7DB',
      strokeWidth: 2,
      dashed: false,
      alpha: 1,
    })
  })

  it('draws an owned token with the colour of the first owner', () => {
    const s = tokenStyle(token({ owners: ['p2', 'p1'] }), players, false)
    expect(s).toMatchObject({ fill: '#2E3D49', stroke: '#2980b9', strokeWidth: 4, dashed: false })
  })

  it('falls back to chalk when the owner is not in the list', () => {
    expect(tokenStyle(token({ owners: ['zz'] }), players, false).stroke).toBe('#ECE7DB')
  })

  it('dashes and fades a hidden token', () => {
    expect(tokenStyle(token({ hidden: true }), players, true)).toMatchObject({
      dashed: true,
      alpha: 0.6,
    })
  })

  it('draws the gm layer in gold, dashed, whoever owns it', () => {
    const s = tokenStyle(token({ layer: 'gm', owners: ['p1'] }), players, true)
    expect(s).toMatchObject({ stroke: '#C9A34E', dashed: true, alpha: 0.6, strokeWidth: 4 })
  })

  it('does not dash the map layer', () => {
    expect(tokenStyle(token({ layer: 'map' }), players, true).dashed).toBe(false)
  })

  it.each([
    ['all', 'goblin', true, 'always'],
    ['all', 'goblin', false, 'always'],
    ['hover', 'goblin', true, 'hover'],
    ['hover', 'goblin', false, 'hover'],
    ['gm', 'goblin', true, 'always'],
    ['gm', 'goblin', false, 'never'],
    ['all', '', true, 'never'],
    ['hover', '', false, 'never'],
    ['gm', '', true, 'never'],
  ] as const)('name visibility %s of "%s" (gm %s) is %s', (vis, name, isGM, want) => {
    expect(tokenStyle(token({ nameVisibility: vis, name }), players, isGM).showName).toBe(want)
  })

  it('takes the upper-case first letter of the name', () => {
    expect(tokenStyle(token({ name: 'goblin' }), players, false).letter).toBe('G')
    expect(tokenStyle(token({ name: 'эльф' }), players, false).letter).toBe('Э')
    expect(tokenStyle(token({ name: '' }), players, false).letter).toBe('')
  })
})

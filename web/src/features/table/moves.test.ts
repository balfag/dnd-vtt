import { describe, expect, it } from 'vitest'
import type { Token } from '../../shared/protocol'
import { createMoves } from './moves'

function token(id: string, x: number, y: number): Token {
  return {
    id,
    sceneId: 's1',
    layer: 'tokens',
    x,
    y,
    w: 1,
    h: 1,
    rotation: 0,
    z: 0,
    name: '',
    nameVisibility: 'all',
    image: null,
    hidden: false,
    locked: false,
    owners: [],
    bars: [],
    conditions: [],
  }
}

describe('createMoves', () => {
  it('shows the pending position after start', () => {
    const m = createMoves()
    m.start('a', { x: 5, y: 6 })
    expect(m.position(token('a', 1, 2))).toEqual({ x: 5, y: 6 })
    expect(m.position(token('b', 1, 2))).toEqual({ x: 1, y: 2 })
  })

  it('shows the server position after confirm', () => {
    const m = createMoves()
    m.start('a', { x: 5, y: 6 })
    m.confirm('a')
    expect(m.position(token('a', 7, 8))).toEqual({ x: 7, y: 8 })
  })

  it('shows the server position after fail', () => {
    const m = createMoves()
    m.start('a', { x: 5, y: 6 })
    m.fail('a')
    expect(m.position(token('a', 1, 2))).toEqual({ x: 1, y: 2 })
  })

  it('replaces the pending move of the same token', () => {
    const m = createMoves()
    m.start('a', { x: 5, y: 6 })
    m.start('a', { x: 9, y: 9 })
    expect(m.position(token('a', 1, 2))).toEqual({ x: 9, y: 9 })
  })
})

import { describe, expect, it } from 'vitest'
import type { Grid, Token } from '../../../shared/protocol'
import { canMove, DRAG_INTERVAL_MS, endDrag, moveDrag, startDrag } from './drag'

const grid: Grid = {
  type: 'square',
  sizePx: 70,
  unitFt: 5,
  offsetX: 0,
  offsetY: 0,
  color: '#9aa8b3',
  visible: true,
}
const bounds = { width: 700, height: 700 }
const one = { w: 1, h: 1 }

function token(over: Partial<Token> = {}): Token {
  return {
    id: 't1',
    sceneId: 's1',
    layer: 'tokens',
    x: 175,
    y: 175,
    w: 1,
    h: 1,
    rotation: 0,
    z: 0,
    name: 'Goblin',
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

describe('moveDrag', () => {
  it('keeps the offset from the grab point', () => {
    // Grabbed 20px right of the centre; moving the pointer by 100px moves the centre by 100px.
    const s = startDrag(token(), { x: 195, y: 175 }, 0)
    const r = moveDrag(s, { x: 295, y: 175 }, 0, grid, one, false, bounds)
    expect(r.state.current).toEqual({ x: 275, y: 175 })
    expect(r.send).toEqual({ x: 275, y: 175 })
  })

  it('snaps the sent position to the grid and not with Alt', () => {
    const s = startDrag(token(), { x: 175, y: 175 }, 0)
    const snapped = moveDrag(s, { x: 270, y: 180 }, 0, grid, one, true, bounds)
    expect(snapped.send).toEqual({ x: 245, y: 175 })
    const free = moveDrag(s, { x: 270, y: 180 }, 0, grid, one, false, bounds)
    expect(free.send).toEqual({ x: 270, y: 180 })
  })

  it('keeps the token inside the scene', () => {
    const s = startDrag(token(), { x: 175, y: 175 }, 0)
    const r = moveDrag(s, { x: 5000, y: -5000 }, 0, grid, one, false, bounds)
    expect(r.state.current).toEqual({ x: 700, y: 0 })
    const snapped = moveDrag(s, { x: 5000, y: -5000 }, 0, grid, one, true, bounds)
    expect(snapped.send).toEqual({ x: 665, y: 35 })
  })

  it('sends at most once per 50 ms', () => {
    let s = startDrag(token(), { x: 175, y: 175 }, 1000)
    const sent: number[] = []
    for (let t = 1000; t <= 1200; t += 10) {
      const r = moveDrag(s, { x: 175 + t - 1000, y: 175 }, t, grid, one, false, bounds)
      s = r.state
      if (r.send !== null) sent.push(t)
    }
    expect(sent).toEqual([1000, 1050, 1100, 1150, 1200])
    for (let i = 1; i < sent.length; i++) {
      expect((sent[i] ?? 0) - (sent[i - 1] ?? 0)).toBeGreaterThanOrEqual(DRAG_INTERVAL_MS)
    }
  })

  it('still tracks the pointer between sends', () => {
    const s0 = startDrag(token(), { x: 175, y: 175 }, 0)
    const s1 = moveDrag(s0, { x: 200, y: 175 }, 0, grid, one, false, bounds).state
    const r = moveDrag(s1, { x: 260, y: 175 }, 10, grid, one, false, bounds)
    expect(r.send).toBeNull()
    expect(r.state.current).toEqual({ x: 260, y: 175 })
  })
})

describe('endDrag', () => {
  it('returns the start when the token never moved, even off the grid', () => {
    const s = startDrag(token({ x: 180, y: 190 }), { x: 180, y: 190 }, 0)
    expect(endDrag(s, grid, one, true, bounds)).toEqual({ x: 180, y: 190 })
  })

  it('returns the same cell centre for a small move inside the cell', () => {
    const s0 = startDrag(token(), { x: 175, y: 175 }, 0)
    const s = moveDrag(s0, { x: 180, y: 170 }, 100, grid, one, true, bounds).state
    expect(endDrag(s, grid, one, true, bounds)).toEqual({ x: 175, y: 175 })
  })

  it('snaps unless asked not to', () => {
    const s0 = startDrag(token(), { x: 175, y: 175 }, 0)
    const s = moveDrag(s0, { x: 330, y: 330 }, 100, grid, one, true, bounds).state
    expect(endDrag(s, grid, one, true, bounds)).toEqual({ x: 315, y: 315 })
    expect(endDrag(s, grid, one, false, bounds)).toEqual({ x: 330, y: 330 })
  })
})

describe('canMove', () => {
  const owned = token({ owners: ['p1'] })
  it.each([
    ['GM, a token without owners', token(), { isGM: true, playerId: null }, true],
    ['GM, a locked token', token({ locked: true }), { isGM: true, playerId: null }, true],
    ['owner', owned, { isGM: false, playerId: 'p1' }, true],
    [
      'owner, locked',
      token({ owners: ['p1'], locked: true }),
      { isGM: false, playerId: 'p1' },
      false,
    ],
    ['somebody else', owned, { isGM: false, playerId: 'p2' }, false],
    ['no player id', owned, { isGM: false, playerId: null }, false],
  ])('%s', (_name, t, me, want) => {
    expect(canMove(t, me)).toBe(want)
  })
})

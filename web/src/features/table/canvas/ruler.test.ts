import { describe, expect, it } from 'vitest'
import type { Grid } from '../../../shared/protocol'
import { cellCenter } from './grid'
import { rulerDistanceFt, rulerPath } from './ruler'

function gridOf(type: Grid['type']): Grid {
  return { type, sizePx: 70, unitFt: 5, offsetX: 0, offsetY: 0, color: '#9aa8b3', visible: true }
}

const square = gridOf('square')
const at = (col: number, row: number) => cellCenter(square, { col, row })

describe('rulerPath', () => {
  it('snaps every waypoint to a cell centre', () => {
    expect(
      rulerPath(square, [
        { x: 10, y: 10 },
        { x: 150, y: 80 },
      ]),
    ).toEqual([at(0, 0), at(2, 1)])
  })

  it('keeps an empty path empty', () => {
    expect(rulerPath(square, [])).toEqual([])
  })
})

describe('rulerDistanceFt', () => {
  it('counts every diagonal cell as 5 ft', () => {
    expect(rulerDistanceFt(square, [at(0, 0), at(5, 5)])).toBe(25)
  })

  it('counts a straight run in cells', () => {
    expect(rulerDistanceFt(square, [at(0, 0), at(4, 0)])).toBe(20)
  })

  it('sums the segments of a path with a waypoint', () => {
    expect(rulerDistanceFt(square, [at(0, 0), at(3, 0), at(3, 4)])).toBe(35)
  })

  it('is 0 for a point or nothing', () => {
    expect(rulerDistanceFt(square, [at(2, 2)])).toBe(0)
    expect(rulerDistanceFt(square, [])).toBe(0)
  })

  it.each(['hexPointy', 'hexFlat'] as const)('counts hex steps on a %s grid', (type) => {
    const g = gridOf(type)
    const neighbour = type === 'hexPointy' ? { col: 1, row: 0 } : { col: 0, row: 1 }
    const far = type === 'hexPointy' ? { col: 3, row: 0 } : { col: 0, row: 3 }
    const origin = cellCenter(g, { col: 0, row: 0 })
    expect(rulerDistanceFt(g, [origin, cellCenter(g, neighbour)])).toBe(5)
    expect(rulerDistanceFt(g, [origin, cellCenter(g, far)])).toBe(15)
  })
})

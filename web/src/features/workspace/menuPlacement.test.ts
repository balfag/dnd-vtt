import { describe, expect, it } from 'vitest'
import { placeMenu } from './menuPlacement'

const viewport = { width: 1000, height: 700 }
const menu = { width: 180, height: 120 }

describe('placeMenu', () => {
  it('keeps the position when the menu fits', () => {
    expect(placeMenu({ x: 400, y: 100 }, 'start', menu, viewport)).toEqual({ left: 400, top: 100 })
    expect(placeMenu({ x: 400, y: 100 }, 'end', menu, viewport)).toEqual({ left: 220, top: 100 })
  })

  it('moves a menu aligned to the end off the left edge to the margin', () => {
    expect(placeMenu({ x: 60, y: 40 }, 'end', menu, viewport)).toEqual({ left: 8, top: 40 })
  })

  it('presses a menu aligned to the start against the right margin', () => {
    expect(placeMenu({ x: 950, y: 40 }, 'start', menu, viewport)).toEqual({ left: 812, top: 40 })
  })

  it('opens above the point when there is no room below', () => {
    expect(placeMenu({ x: 400, y: 650 }, 'start', menu, viewport)).toEqual({ left: 400, top: 522 })
  })

  it('sticks to the top margin when the menu is taller than the window', () => {
    expect(placeMenu({ x: 400, y: 100 }, 'start', { width: 180, height: 900 }, viewport)).toEqual({
      left: 400,
      top: 8,
    })
  })
})

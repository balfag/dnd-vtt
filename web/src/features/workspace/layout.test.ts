import type { SerializedDockview } from 'dockview-react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { layoutKey, loadLayout, sanitizeLayout, saveLayout } from './layout'

interface FakeStorage {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
  removeItem: (key: string) => void
}

/** stubStorage puts a storage in place of the browser's; the values are kept in the returned map. */
function stubStorage(overrides: Partial<FakeStorage> = {}): Map<string, string> {
  const values = new Map<string, string>()
  const storage: FakeStorage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      values.set(key, value)
    },
    removeItem: (key) => {
      values.delete(key)
    },
    ...overrides,
  }
  vi.stubGlobal('window', { localStorage: storage })
  return values
}

afterEach(() => {
  vi.unstubAllGlobals()
})

type Panels = SerializedDockview['panels']

function panelStates(...kinds: string[]): Panels {
  return Object.fromEntries(kinds.map((k) => [k, { id: k, contentComponent: k, title: k }]))
}

/** layoutOf is a layout of the canvas on the left and the groups of views on the right. */
function layoutOf(
  groups: string[][],
  extra: Partial<SerializedDockview> = {},
  kinds = ['scene', ...groups.flat()],
): SerializedDockview {
  return {
    grid: {
      height: 600,
      width: 1000,
      orientation: 'HORIZONTAL',
      root: {
        type: 'branch',
        data: [
          { type: 'leaf', size: 700, data: { id: 'g0', views: ['scene'], activeView: 'scene' } },
          ...groups.map((views, i) => ({
            type: 'leaf' as const,
            size: 300,
            data: { id: `g${String(i + 1)}`, views, activeView: views[0] },
          })),
        ],
      },
    },
    panels: panelStates(...kinds),
    ...extra,
  } as unknown as SerializedDockview
}

const everything = () => true

describe('layoutKey', () => {
  it('names the account, the campaign and the screen', () => {
    expect(layoutKey('acc1', 'camp2', 'table')).toBe('dndvtt.layout.v1.acc1.camp2.table')
    expect(layoutKey('acc1', 'camp2', 'wiki')).toBe('dndvtt.layout.v1.acc1.camp2.wiki')
  })
})

describe('loadLayout', () => {
  it('returns null when there is no key', () => {
    stubStorage()
    expect(loadLayout('k')).toBeNull()
  })

  it('returns null for broken JSON', () => {
    const values = stubStorage()
    values.set('k', '{not json')
    expect(loadLayout('k')).toBeNull()
  })

  it('returns null for another version', () => {
    const values = stubStorage()
    values.set('k', JSON.stringify({ v: 2, layout: layoutOf([['chat']]) }))
    expect(loadLayout('k')).toBeNull()
  })

  it('returns null when reading throws', () => {
    stubStorage({
      getItem: () => {
        throw new Error('blocked')
      },
    })
    expect(loadLayout('k')).toBeNull()
  })

  it('returns the saved layout', () => {
    const values = stubStorage()
    const layout = layoutOf([['chat']])
    values.set('k', JSON.stringify({ v: 1, layout }))
    expect(loadLayout('k')).toEqual(layout)
  })
})

describe('saveLayout', () => {
  it('stores the version with the layout', () => {
    const values = stubStorage()
    const layout = layoutOf([['chat']])
    saveLayout('k', layout)
    expect(JSON.parse(values.get('k') ?? 'null')).toEqual({ v: 1, layout })
  })

  it('does not throw when writing throws', () => {
    stubStorage({
      setItem: () => {
        throw new Error('quota')
      },
    })
    expect(() => {
      saveLayout('k', layoutOf([['chat']]))
    }).not.toThrow()
  })
})

describe('sanitizeLayout', () => {
  it('keeps a layout that fits', () => {
    const layout = layoutOf([['chat', 'combat']])
    expect(sanitizeLayout(layout, everything)).toEqual(layout)
  })

  it('drops a panel of an unknown kind', () => {
    const layout = layoutOf([['chat', 'nonsense']])
    const out = sanitizeLayout(layout, (k) => k !== 'nonsense')
    expect(Object.keys(out?.panels ?? {}).sort()).toEqual(['chat', 'scene'])
  })

  it('drops a panel the role does not have', () => {
    const layout = layoutOf([['chat', 'scenes']])
    const out = sanitizeLayout(layout, (k) => k !== 'scenes')
    expect(Object.keys(out?.panels ?? {}).sort()).toEqual(['chat', 'scene'])
    expect(JSON.stringify(out?.grid)).not.toContain('scenes')
  })

  it('drops a group left empty and moves the active view', () => {
    const layout = layoutOf([['scenes'], ['chat', 'scenes2']], {}, [
      'scene',
      'scenes',
      'chat',
      'scenes2',
    ])
    const out = sanitizeLayout(layout, (k) => !k.startsWith('scenes'))
    const root = out?.grid.root as unknown as {
      data: { data: { id: string; activeView: string } }[]
    }
    expect(root.data.map((n) => n.data.id)).toEqual(['g0', 'g2'])
    expect(root.data[1]?.data.activeView).toBe('chat')
  })

  it('returns null when nothing is left', () => {
    expect(sanitizeLayout(layoutOf([['chat']]), () => false)).toBeNull()
  })

  it('brings the panels of popout windows back to the first group without the canvas', () => {
    const layout = layoutOf(
      [['chat']],
      {
        popoutGroups: [
          { data: { id: 'p1', views: ['combat'], activeView: 'combat' }, position: null },
        ],
      },
      ['scene', 'chat', 'combat'],
    )
    const out = sanitizeLayout(layout, everything)
    const root = out?.grid.root as unknown as { data: { data: { views: string[] } }[] }
    expect(root.data[1]?.data.views).toEqual(['chat', 'combat'])
    expect(out?.popoutGroups).toBeUndefined()
    expect(Object.keys(out?.panels ?? {}).sort()).toEqual(['chat', 'combat', 'scene'])
  })
})

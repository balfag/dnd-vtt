import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { SceneView } from '../../shared/protocol'
import { RulerTool, RULER_SEND_EVERY_MS } from './RulerTool'
import { DEFAULT_DRAW, DEFAULT_TEMPLATE, type ToolEnv, type ToolPointer } from './toolState'

const scene: SceneView = {
  id: 's1',
  name: 'Crypt',
  width: 2000,
  height: 2000,
  grid: {
    type: 'square',
    sizePx: 70,
    unitFt: 5,
    offsetX: 0,
    offsetY: 0,
    color: '#9aa8b3',
    visible: true,
  },
  background: null,
  fog: { enabled: false, ops: [] },
  tokens: [],
  drawings: [],
}

interface Sent {
  type: string
  data: unknown
  at: number
}

function setup() {
  const sent: Sent[] = []
  let changes = 0
  const env: ToolEnv = {
    scene: () => scene,
    zoom: () => 1,
    drawOptions: () => DEFAULT_DRAW,
    templateOptions: () => DEFAULT_TEMPLATE,
    myColor: () => '#e53935',
    transient: (type, data) => {
      sent.push({ type, data, at: Date.now() })
    },
    command: () => Promise.resolve(undefined),
    announce: () => undefined,
    t: (key) => key,
    changed: () => {
      changes++
    },
  }
  const tool = new RulerTool(env)
  return { tool, sent, changes: () => changes }
}

function at(col: number, row: number): ToolPointer {
  const world = { x: col * 70 + 35, y: row * 70 + 35 }
  return { world, screen: world, shiftKey: false }
}

describe('RulerTool', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(100_000)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('measures a drag in feet along the cells', () => {
    const { tool } = setup()
    tool.down(at(0, 0), 1)
    tool.move(at(3, 3))
    expect(tool.view()?.ft).toBe(15)
    tool.up(at(3, 3), 1)
    expect(tool.has()).toBe(true)
    expect(tool.view()?.ft).toBe(15)
  })

  it('leaves the ruler where the drag ended and ignores the pointer after that', () => {
    const { tool } = setup()
    tool.down(at(0, 0), 1)
    tool.move(at(4, 0))
    tool.up(at(4, 0), 1)
    tool.move(at(9, 0))
    expect(tool.view()?.ft).toBe(20)
  })

  it('adds a waypoint on a click and pulls the next segment from it', () => {
    const { tool } = setup()
    tool.down(at(0, 0), 1)
    tool.up(at(0, 0), 1)
    tool.move(at(3, 0))
    tool.down(at(3, 0), 2)
    tool.up(at(3, 0), 2)
    tool.move(at(3, 4))
    expect(tool.view()?.ft).toBe(35)
    expect(tool.view()?.path).toHaveLength(3)
  })

  it('sends at most 20 updates a second, the latest one last', () => {
    const { tool, sent } = setup()
    tool.down(at(0, 0), 1)
    for (let i = 1; i <= 30; i++) {
      vi.advanceTimersByTime(10)
      tool.move(at(i % 20, 0))
    }
    vi.advanceTimersByTime(RULER_SEND_EVERY_MS)
    const updates = sent.filter((m) => m.type === 'ruler.update')
    // Moves come every 10 ms, updates never closer than 50 ms.
    expect(updates.length).toBeGreaterThan(2)
    for (let i = 1; i < updates.length; i++) {
      expect((updates[i]?.at ?? 0) - (updates[i - 1]?.at ?? 0)).toBeGreaterThanOrEqual(
        RULER_SEND_EVERY_MS,
      )
    }
    const last = updates[updates.length - 1]?.data as { points: [number, number][] }
    expect(last.points[last.points.length - 1]).toEqual([(30 % 20) * 70 + 35, 35])
  })

  it('sends the cells it sees, within the limit of 50 points', () => {
    const { tool, sent } = setup()
    tool.down(at(0, 0), 1)
    tool.up(at(0, 0), 1)
    for (let i = 1; i <= 60; i++) {
      vi.advanceTimersByTime(RULER_SEND_EVERY_MS)
      tool.move(at(i % 25, 0))
      tool.down(at(i % 25, 0), 1)
      tool.up(at(i % 25, 0), 1)
    }
    vi.advanceTimersByTime(RULER_SEND_EVERY_MS)
    for (const m of sent.filter((s) => s.type === 'ruler.update')) {
      expect((m.data as { points: unknown[] }).points.length).toBeLessThanOrEqual(50)
    }
  })

  it('tells the others when the ruler is removed, once', () => {
    const { tool, sent } = setup()
    tool.down(at(0, 0), 1)
    tool.move(at(2, 0))
    tool.up(at(2, 0), 1)
    tool.clear()
    tool.clear()
    expect(sent.filter((m) => m.type === 'ruler.clear').map((m) => m.data)).toEqual([
      { sceneId: 's1' },
    ])
    expect(tool.has()).toBe(false)
    expect(tool.view()).toBeNull()
  })

  it('has nothing to clear on the others screens before the first update', () => {
    const { tool, sent } = setup()
    tool.clear()
    expect(sent).toEqual([])
  })
})

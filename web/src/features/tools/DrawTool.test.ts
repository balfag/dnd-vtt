import { describe, expect, it } from 'vitest'
import type { SceneView } from '../../shared/protocol'
import { clampTextSize, DrawTool, freehandPoints, MAX_FREEHAND_POINTS, validText } from './DrawTool'
import { TemplateTool } from './TemplateTool'
import {
  DEFAULT_DRAW,
  DEFAULT_TEMPLATE,
  type DrawOptions,
  type TemplateOptions,
  type ToolEnv,
  type ToolPointer,
} from './toolState'

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

function setup(draw: Partial<DrawOptions> = {}, template: Partial<TemplateOptions> = {}) {
  const commands: { type: string; data: unknown }[] = []
  const env: ToolEnv = {
    scene: () => scene,
    zoom: () => 2,
    drawOptions: () => ({ ...DEFAULT_DRAW, ...draw }),
    templateOptions: () => ({ ...DEFAULT_TEMPLATE, ...template }),
    myColor: () => '#1e88e5',
    transient: () => undefined,
    command: (type, data) => {
      commands.push({ type, data })
      return Promise.resolve(undefined)
    },
    announce: () => undefined,
    t: (key) => key,
    changed: () => undefined,
  }
  return { env, commands, draw: new DrawTool(env), template: new TemplateTool(env) }
}

const at = (x: number, y: number): ToolPointer => ({
  world: { x, y },
  screen: { x: x * 2, y: y * 2 },
  shiftKey: false,
})

describe('freehandPoints', () => {
  it('simplifies a straight stroke to its ends', () => {
    const line = Array.from({ length: 100 }, (_, i) => ({ x: i, y: i }))
    expect(freehandPoints(line, 1)).toEqual([
      [0, 0],
      [99, 99],
    ])
  })

  it('refuses a speck', () => {
    expect(freehandPoints([{ x: 5, y: 5 }], 1)).toBeNull()
  })

  it('keeps no more than 2000 points', () => {
    const wave = Array.from({ length: 6000 }, (_, i) => ({
      x: i,
      y: i % 2 === 0 ? 0 : 50,
    }))
    expect(freehandPoints(wave, 1)?.length).toBe(MAX_FREEHAND_POINTS)
  })
})

describe('text', () => {
  it('takes 1 to 500 characters', () => {
    expect(validText('')).toBe(false)
    expect(validText('a')).toBe(true)
    expect(validText('a'.repeat(500))).toBe(true)
    expect(validText('a'.repeat(501))).toBe(false)
  })

  it('keeps the size between 8 and 200', () => {
    expect(clampTextSize(1)).toBe(8)
    expect(clampTextSize(24.4)).toBe(24)
    expect(clampTextSize(999)).toBe(200)
  })
})

describe('DrawTool', () => {
  it('sends a rectangle from corner to corner with the colour of the player', () => {
    const { draw, commands } = setup({ kind: 'rect' })
    draw.down(at(100, 80), 1)
    draw.up(at(40, 20), 1)
    expect(commands).toEqual([
      {
        type: 'drawing.create',
        data: {
          sceneId: 's1',
          kind: 'rect',
          data: { x: 40, y: 20, w: 60, h: 60 },
          color: '#1e88e5',
        },
      },
    ])
  })

  it('draws a circle from its centre and uses the chosen colour', () => {
    const { draw, commands } = setup({ kind: 'circle', color: '#43a047' })
    draw.down(at(100, 100), 1)
    draw.up(at(130, 140), 1)
    expect(commands[0]?.data).toEqual({
      sceneId: 's1',
      kind: 'circle',
      data: { x: 100, y: 100, r: 50 },
      color: '#43a047',
    })
  })

  it('sends nothing for a stray click', () => {
    const { draw, commands } = setup({ kind: 'rect' })
    draw.down(at(100, 100), 1)
    draw.up(at(100, 100), 1)
    const circle = setup({ kind: 'circle' })
    circle.draw.down(at(100, 100), 1)
    circle.draw.up(at(100, 100), 1)
    expect(commands).toEqual([])
    expect(circle.commands).toEqual([])
  })

  it('sends a freehand stroke as simplified points', () => {
    const { draw, commands } = setup({ kind: 'freehand' })
    draw.down(at(0, 0), 1)
    for (let i = 1; i <= 50; i++) draw.move(at(i, i), 1)
    draw.up(at(50, 50), 1)
    expect((commands[0]?.data as { data: { points: unknown[] } }).data.points).toEqual([
      [0, 0],
      [50, 50],
    ])
  })

  it('creates a text only when it is valid', () => {
    const { draw, commands } = setup({ kind: 'text', textSize: 30 })
    expect(draw.createText({ x: 5, y: 6 }, '')).toBe(false)
    expect(draw.createText({ x: 5, y: 6 }, 'Hi')).toBe(true)
    expect(commands).toHaveLength(1)
    expect(commands[0]?.data).toEqual({
      sceneId: 's1',
      kind: 'text',
      data: { x: 5, y: 6, text: 'Hi', size: 30 },
      color: '#1e88e5',
    })
  })

  it('asks for the words of a text instead of drawing', () => {
    const { draw } = setup({ kind: 'text' })
    expect(draw.down(at(1, 1), 1)).toBe('text')
    expect(draw.drawing()).toBe(false)
  })

  it('ignores another pointer and drops a cancelled stroke', () => {
    const { draw, commands } = setup({ kind: 'rect' })
    draw.down(at(0, 0), 1)
    draw.up(at(50, 50), 2)
    expect(commands).toEqual([])
    draw.cancel()
    expect(draw.drawing()).toBe(false)
  })
})

describe('TemplateTool', () => {
  it('places a template pointing where the drag went', () => {
    const { template, commands } = setup({}, { shape: 'cone', size: 15 })
    template.down(at(100, 100), 1)
    template.up(at(100, 200), 1)
    expect(commands[0]).toEqual({
      type: 'drawing.create',
      data: {
        sceneId: 's1',
        kind: 'template',
        data: { shape: 'cone', x: 100, y: 100, size: 15, direction: 90 },
        color: '#4fa58e',
      },
    })
  })

  it('points a click along the X axis', () => {
    const { template, commands } = setup({}, { shape: 'line', size: 30 })
    template.down(at(10, 10), 1)
    template.up(at(10, 10), 1)
    expect((commands[0]?.data as { data: { direction: number } }).data.direction).toBe(0)
  })

  it('does not turn a sphere', () => {
    const { template, commands } = setup({}, { shape: 'sphere', size: 20 })
    template.down(at(10, 10), 1)
    template.up(at(200, 300), 1)
    expect((commands[0]?.data as { data: { direction: number } }).data.direction).toBe(0)
  })

  it('shows a preview while the pointer is down', () => {
    const { template } = setup({}, { shape: 'cube', size: 15 })
    expect(template.preview('p1')).toBeNull()
    template.down(at(10, 10), 1)
    expect(template.preview('p1')?.kind).toBe('template')
    template.cancel()
    expect(template.preview('p1')).toBeNull()
  })
})

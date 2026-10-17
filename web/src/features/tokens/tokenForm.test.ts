import { describe, expect, it } from 'vitest'
import type { PlayerInfo, Token } from '../../shared/protocol'
import {
  buildTokenCreate,
  buildTokenUpdate,
  formFromToken,
  newBar,
  ownerChoices,
  sizeOf,
  tokenNameFromFile,
  validateTokenForm,
  zOrder,
  type TokenForm,
} from './tokenForm'

const players: PlayerInfo[] = [
  { id: 'gm1', displayName: 'GM', color: '#c9a34e', role: 'gm', online: true },
  { id: 'p1', displayName: 'Ann', color: '#4fa58e', role: 'player', online: true },
  { id: 'p2', displayName: 'Bob', color: '#c25468', role: 'player', online: false },
]

const token: Token = {
  id: 't1',
  sceneId: 's1',
  layer: 'tokens',
  x: 105,
  y: 105,
  w: 1,
  h: 1,
  rotation: 0,
  z: 3,
  name: 'Goblin',
  nameVisibility: 'hover',
  image: { assetId: 'a1', url: '/a1' },
  hidden: false,
  locked: false,
  owners: ['p1'],
  bars: [{ key: 'hp', value: 7, max: 7, visibility: 'all' }],
  conditions: ['prone'],
}

function form(patch: Partial<TokenForm> = {}): TokenForm {
  return { ...formFromToken(token, players), ...patch }
}

describe('formFromToken', () => {
  it('maps a preset size and keeps the custom one as text', () => {
    expect(form().size).toBe('medium')
    expect(formFromToken({ ...token, w: 2, h: 2 }, players).size).toBe('large')
    expect(formFromToken({ ...token, w: 0.5, h: 0.5 }, players).size).toBe('tiny')
    const custom = formFromToken({ ...token, w: 2, h: 3 }, players)
    expect(custom.size).toBe('custom')
    expect(custom.width).toBe('2')
    expect(custom.height).toBe('3')
  })

  it('drops owners who are not players of the campaign any more', () => {
    expect(formFromToken({ ...token, owners: ['p1', 'gone', 'gm1'] }, players).owners).toEqual([
      'p1',
    ])
  })
})

describe('ownerChoices', () => {
  it('lists the players without the GM', () => {
    expect(ownerChoices(players).map((p) => p.id)).toEqual(['p1', 'p2'])
  })
})

describe('sizeOf', () => {
  it.each([
    ['tiny', 0.5],
    ['medium', 1],
    ['large', 2],
    ['huge', 3],
    ['gargantuan', 4],
  ] as const)('%s is %s cells on both edges', (size, cells) => {
    expect(sizeOf(form({ size }))).toEqual({ w: cells, h: cells })
  })

  it('reads the edges of a custom size', () => {
    expect(sizeOf(form({ size: 'custom', width: '2.5', height: '1' }))).toEqual({ w: 2.5, h: 1 })
  })
})

describe('validateTokenForm', () => {
  it('accepts the form of an existing token', () => {
    expect(validateTokenForm(form())).toEqual({})
  })

  it('limits the name to 100 characters', () => {
    expect(validateTokenForm(form({ name: 'я'.repeat(100) }))).toEqual({})
    expect(validateTokenForm(form({ name: 'я'.repeat(101) })).name).toBeDefined()
  })

  it('checks the custom size between 0.25 and 20 cells, and ignores it for a preset', () => {
    const custom = (width: string, height: string) =>
      validateTokenForm(form({ size: 'custom', width, height }))
    expect(custom('0.25', '20')).toEqual({})
    expect(custom('0.2', '1').width).toBeDefined()
    expect(custom('1', '20.5').height).toBeDefined()
    expect(custom('', '1').width).toBeDefined()
    expect(custom('abc', '1').width).toBeDefined()
    expect(validateTokenForm(form({ size: 'large', width: 'abc' }))).toEqual({})
  })

  it('takes any number as the rotation', () => {
    expect(validateTokenForm(form({ rotation: '-450.5' }))).toEqual({})
    expect(validateTokenForm(form({ rotation: '' })).rotation).toBeDefined()
    expect(validateTokenForm(form({ rotation: 'x' })).rotation).toBeDefined()
  })

  it('accepts bar keys of 1 to 16 lower case letters, digits and underscores', () => {
    const key = (k: string) =>
      validateTokenForm(form({ bars: [{ key: k, value: '1', max: '1', visibility: 'all' }] }))[
        'bar.0.key'
      ]
    expect(key('hp')).toBeUndefined()
    expect(key('a_1')).toBeUndefined()
    expect(key('x'.repeat(16))).toBeUndefined()
    expect(key('HP')).toBeDefined()
    expect(key('')).toBeDefined()
    expect(key('x'.repeat(17))).toBeDefined()
    expect(key('hit points')).toBeDefined()
    expect(key('hp-1')).toBeDefined()
  })

  it('refuses a bar key that is taken', () => {
    const bar = { key: 'hp', value: '1', max: '1', visibility: 'all' as const }
    const errors = validateTokenForm(form({ bars: [bar, { ...bar }] }))
    expect(errors['bar.0.key']).toBeUndefined()
    expect(errors['bar.1.key']).toBeDefined()
  })

  it('wants whole numbers in the bar, and a maximum of at least 1', () => {
    const bar = (value: string, max: string) =>
      validateTokenForm(form({ bars: [{ key: 'hp', value, max, visibility: 'gm' }] }))
    expect(bar('-9999', '1')).toEqual({})
    expect(bar('99999', '99999')).toEqual({})
    expect(bar('-10000', '1')['bar.0.value']).toBeDefined()
    expect(bar('100000', '1')['bar.0.value']).toBeDefined()
    expect(bar('1.5', '1')['bar.0.value']).toBeDefined()
    expect(bar('1', '0')['bar.0.max']).toBeDefined()
    expect(bar('1', '100000')['bar.0.max']).toBeDefined()
    expect(bar('1', '')['bar.0.max']).toBeDefined()
  })

  it('allows five bars and no more', () => {
    const rows = (n: number) =>
      Array.from({ length: n }, (_, i) => ({
        key: `b${String(i)}`,
        value: '1',
        max: '1',
        visibility: 'all' as const,
      }))
    expect(validateTokenForm(form({ bars: rows(5) }))).toEqual({})
    expect(validateTokenForm(form({ bars: rows(6) })).bars).toBeDefined()
  })
})

describe('newBar', () => {
  it('names the first bar hp and leaves the next ones to be named', () => {
    const first = newBar([])
    expect(first.key).toBe('hp')
    expect(validateTokenForm(form({ bars: [first] }))).toEqual({})
    expect(newBar([first]).key).toBe('')
  })
})

describe('buildTokenUpdate', () => {
  it('is null when nothing changed', () => {
    expect(buildTokenUpdate(token, form())).toBeNull()
  })

  it('sends only the fields that changed', () => {
    expect(buildTokenUpdate(token, form({ name: ' Boss ' }))).toEqual({
      tokenId: 't1',
      name: 'Boss',
    })
    expect(buildTokenUpdate(token, form({ hidden: true, locked: true }))).toEqual({
      tokenId: 't1',
      hidden: true,
      locked: true,
    })
    expect(buildTokenUpdate(token, form({ layer: 'gm', rotation: '90' }))).toEqual({
      tokenId: 't1',
      layer: 'gm',
      rotation: 90,
    })
    expect(buildTokenUpdate(token, form({ nameVisibility: 'gm' }))).toEqual({
      tokenId: 't1',
      nameVisibility: 'gm',
    })
  })

  it('sends both edges of a changed size, only the changed edge of a custom one', () => {
    expect(buildTokenUpdate(token, form({ size: 'large' }))).toEqual({ tokenId: 't1', w: 2, h: 2 })
    expect(buildTokenUpdate(token, form({ size: 'custom', width: '1', height: '3' }))).toEqual({
      tokenId: 't1',
      h: 3,
    })
  })

  it('sends imageAssetId null when the image is removed, and the id when it is replaced', () => {
    expect(buildTokenUpdate(token, form({ imageAssetId: null }))).toEqual({
      tokenId: 't1',
      imageAssetId: null,
    })
    expect(buildTokenUpdate(token, form({ imageAssetId: 'a2' }))).toEqual({
      tokenId: 't1',
      imageAssetId: 'a2',
    })
    expect(
      buildTokenUpdate({ ...token, image: null }, { ...form(), imageAssetId: null }),
    ).toBeNull()
  })

  it('sends owners, bars and conditions whole', () => {
    const update = buildTokenUpdate(
      token,
      form({
        owners: ['p1', 'p2'],
        bars: [
          { key: 'hp', value: '5', max: '7', visibility: 'owners' },
          { key: 'ac', value: '12', max: '12', visibility: 'gm' },
        ],
        conditions: ['prone', 'blinded'],
      }),
    )
    expect(update).toEqual({
      tokenId: 't1',
      owners: ['p1', 'p2'],
      bars: [
        { key: 'hp', value: 5, max: 7, visibility: 'owners' },
        { key: 'ac', value: 12, max: 12, visibility: 'gm' },
      ],
      conditions: ['prone', 'blinded'],
    })
  })

  it('clears owners and conditions with empty lists', () => {
    expect(buildTokenUpdate(token, form({ owners: [], conditions: [], bars: [] }))).toEqual({
      tokenId: 't1',
      owners: [],
      conditions: [],
      bars: [],
    })
  })

  it('does not count another order of the conditions as a change', () => {
    const two = { ...token, conditions: ['prone', 'blinded'] }
    expect(
      buildTokenUpdate(two, formFromToken({ ...two, conditions: ['blinded', 'prone'] }, players)),
    ).toBeNull()
  })

  it('sends the owners without the ones who left', () => {
    const stale = { ...token, owners: ['p1', 'gone'] }
    expect(buildTokenUpdate(stale, formFromToken(stale, players))).toEqual({
      tokenId: 't1',
      owners: ['p1'],
    })
  })
})

describe('tokenNameFromFile', () => {
  it('cuts the extension', () => {
    expect(tokenNameFromFile('goblin.png')).toBe('goblin')
    expect(tokenNameFromFile('goblin.archer.webp')).toBe('goblin.archer')
    expect(tokenNameFromFile('goblin')).toBe('goblin')
    expect(tokenNameFromFile('.png')).toBe('.png')
  })

  it('cuts a long name to 100 characters', () => {
    expect(Array.from(tokenNameFromFile(`${'я'.repeat(150)}.png`))).toHaveLength(100)
  })
})

describe('buildTokenCreate', () => {
  it('makes one cell on the tokens layer, named after the image', () => {
    expect(
      buildTokenCreate('s1', { x: 35, y: 105 }, { assetId: 'a1', originalName: 'orc.png' }),
    ).toEqual({
      sceneId: 's1',
      layer: 'tokens',
      x: 35,
      y: 105,
      w: 1,
      h: 1,
      name: 'orc',
      imageAssetId: 'a1',
    })
  })

  it('has no name and no image without an image', () => {
    const create = buildTokenCreate('s1', { x: 35, y: 35 }, null)
    expect(create.name).toBe('')
    expect('imageAssetId' in create).toBe(false)
  })
})

describe('zOrder', () => {
  const others: Token[] = [
    { ...token, id: 'a', z: 1 },
    { ...token, id: 'b', z: 9 },
    { ...token, id: 'c', z: 100, layer: 'gm' },
    { ...token, id: 'd', z: -50, layer: 'map' },
  ]

  it('goes above the highest or below the lowest z of the same layer', () => {
    expect(zOrder(others[0] ?? token, others, 'front')).toBe(10)
    expect(zOrder(others[1] ?? token, others, 'back')).toBe(0)
  })

  it('does not look at the other layers', () => {
    expect(zOrder(others[0] ?? token, others, 'back')).toBe(0)
    expect(zOrder(others[2] ?? token, others, 'front')).toBe(101)
  })
})

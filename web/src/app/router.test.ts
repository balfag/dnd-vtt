import { describe, expect, it } from 'vitest'
import { matchRoute } from './router'

describe('matchRoute', () => {
  it.each([
    ['/', { name: 'home' }],
    ['/setup', { name: 'setup' }],
    ['/login', { name: 'login' }],
    ['/login/', { name: 'login' }],
    ['/table', { name: 'table' }],
    ['/table/', { name: 'table' }],
    ['/play/abc_DEF-1', { name: 'play', token: 'abc_DEF-1' }],
    ['/play/abc_DEF-1/', { name: 'play', token: 'abc_DEF-1' }],
    ['/reset/x', { name: 'reset', token: 'x' }],
  ])('%s', (pathname, want) => {
    expect(matchRoute(pathname)).toEqual(want)
  })

  it.each([
    '/play/',
    '/play',
    '/play/a/b',
    '/reset/',
    '/reset/a b',
    '/play/a.b',
    '/nope',
    '//',
    '/setup/x',
  ])('%s is not found', (pathname) => {
    expect(matchRoute(pathname)).toEqual({ name: 'notFound' })
  })
})

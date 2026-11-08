import { describe, expect, it } from 'vitest'
import { isNetAddresses, withHost } from './net'

describe('isNetAddresses', () => {
  it('accepts a valid object', () => {
    expect(isNetAddresses({ scheme: 'https', port: 7777, hosts: ['192.168.1.5'] })).toBe(true)
  })

  it('rejects a missing hosts', () => {
    expect(isNetAddresses({ scheme: 'https', port: 7777 })).toBe(false)
  })

  it('rejects hosts that are not strings', () => {
    expect(isNetAddresses({ scheme: 'https', port: 7777, hosts: [1] })).toBe(false)
  })

  it('rejects an unknown scheme', () => {
    expect(isNetAddresses({ scheme: 'ftp', port: 7777, hosts: [] })).toBe(false)
  })
})

describe('withHost', () => {
  it('replaces the host and keeps port and path', () => {
    expect(withHost('https://localhost:7777/play/abc', '192.168.1.5')).toBe(
      'https://192.168.1.5:7777/play/abc',
    )
  })

  it('puts an IPv6 host in brackets', () => {
    expect(withHost('https://localhost:7777/play/abc', '2001:db8::1')).toBe(
      'https://[2001:db8::1]:7777/play/abc',
    )
  })

  it('returns a broken link unchanged', () => {
    expect(withHost('not a link', '192.168.1.5')).toBe('not a link')
  })
})

import { describe, expect, it } from 'vitest'
import { pickLanHost } from './lanHost'

const hosts = ['192.168.1.5', '10.0.0.7']
const link = 'https://192.168.1.5:7777/play/abc'

describe('pickLanHost', () => {
  it('has nothing to choose with one address', () => {
    expect(pickLanHost(['192.168.1.5'], link, null)).toBeNull()
  })

  it('has nothing to choose without a link', () => {
    expect(pickLanHost(hosts, null, null)).toBeNull()
  })

  it('has nothing to choose with a broken link', () => {
    expect(pickLanHost(hosts, 'not a link', null)).toBeNull()
  })

  it('defaults to the link host', () => {
    expect(pickLanHost(hosts, link, null)).toBe('192.168.1.5')
  })

  it('prefers the stored address from the list', () => {
    expect(pickLanHost(hosts, link, '10.0.0.7')).toBe('10.0.0.7')
  })

  it('ignores a stored address that is not in the list', () => {
    expect(pickLanHost(hosts, link, '172.16.0.1')).toBe('192.168.1.5')
  })

  it('has nothing to choose when the link host is not in the list', () => {
    expect(pickLanHost(hosts, 'https://localhost:7777/play/abc', null)).toBeNull()
  })

  it('compares IPv6 hosts without brackets', () => {
    expect(pickLanHost(['192.168.1.5', '2001:db8::1'], 'https://[2001:db8::1]:7777/p', null)).toBe(
      '2001:db8::1',
    )
  })
})

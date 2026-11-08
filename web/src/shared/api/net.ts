import { ApiError, apiRequest } from './client'

export interface NetAddresses {
  scheme: 'http' | 'https'
  port: number
  hosts: string[]
}

export function isNetAddresses(v: unknown): v is NetAddresses {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) return false
  const r = v as Record<string, unknown>
  return (
    (r.scheme === 'http' || r.scheme === 'https') &&
    typeof r.port === 'number' &&
    Array.isArray(r.hosts) &&
    r.hosts.every((h) => typeof h === 'string')
  )
}

/** getNetAddresses asks the server for LAN addresses (GM only). */
export async function getNetAddresses(): Promise<NetAddresses> {
  const value = await apiRequest('GET', '/api/net/addresses', { guard: isNetAddresses })
  if (value === undefined) {
    throw new ApiError(200, 'unknown')
  }
  return value
}

/**
 * withHost returns link with its host replaced by host (port and path kept).
 * IPv6 hosts (containing ':') are put in brackets. An invalid link is returned unchanged.
 */
export function withHost(link: string, host: string): string {
  try {
    const u = new URL(link)
    u.hostname = host.includes(':') ? `[${host}]` : host
    return u.toString()
  } catch {
    return link
  }
}

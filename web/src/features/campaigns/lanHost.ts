/** linkHost is the host of link without IPv6 brackets, or null for an invalid link. */
function linkHost(link: string | null): string | null {
  if (link === null) return null
  try {
    return new URL(link).hostname.replace(/^\[|\]$/g, '')
  } catch {
    return null
  }
}

/**
 * pickLanHost returns the address to show, or null when there is nothing to choose:
 * hosts.length <= 1, link is null or invalid, or the link host is not in hosts.
 * Otherwise stored if it is in hosts, else the link host (IPv6 without brackets).
 */
export function pickLanHost(
  hosts: string[],
  link: string | null,
  stored: string | null,
): string | null {
  if (hosts.length <= 1) return null
  const own = linkHost(link)
  if (own === null || !hosts.includes(own)) return null
  return stored !== null && hosts.includes(stored) ? stored : own
}

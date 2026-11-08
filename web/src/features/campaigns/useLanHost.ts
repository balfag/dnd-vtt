import { useCallback, useEffect, useState } from 'react'
import { getNetAddresses } from '../../shared/api/net'
import { pickLanHost } from './lanHost'

export interface LanHost {
  /** hosts is the server list; [] while loading, on error or with one address or less. */
  hosts: string[]
  /** host is the chosen address or null when there is nothing to choose. */
  host: string | null
  setHost: (host: string) => void
}

const storageKey = 'dndvtt.lanHost'

function readStored(): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

/** useLanHost loads /api/net/addresses once and remembers the choice in localStorage. */
export function useLanHost(link: string | null): LanHost {
  const [all, setAll] = useState<string[]>([])
  const [stored, setStored] = useState<string | null>(readStored)

  useEffect(() => {
    let cancelled = false
    getNetAddresses().then(
      ({ hosts }) => {
        if (!cancelled) setAll(hosts)
      },
      () => {
        // No list (an older server, a network error): the link stays as it is.
      },
    )
    return () => {
      cancelled = true
    }
  }, [])

  const setHost = useCallback((host: string) => {
    setStored(host)
    try {
      localStorage.setItem(storageKey, host)
    } catch {
      // Private mode: the choice lives until the page closes.
    }
  }, [])

  const host = pickLanHost(all, link, stored)
  return { hosts: host === null ? [] : all, host, setHost }
}

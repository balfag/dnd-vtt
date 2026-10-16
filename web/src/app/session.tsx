import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { ApiError } from '../shared/api/client'
import { getMe, postLogout, type Me } from '../shared/api/auth'
import { navigate } from './router'

type SessionState = { status: 'loading' } | { status: 'anonymous' } | { status: 'signedIn'; me: Me }

interface SessionValue {
  state: SessionState
  setMe: (me: Me) => void
  refresh: () => Promise<void>
  logout: () => Promise<void>
}

const SessionContext = createContext<SessionValue | null>(null)

/** SessionProvider asks the server who is signed in and shares the answer. */
export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<SessionState>({ status: 'loading' })

  const refresh = useCallback(async () => {
    try {
      setState({ status: 'signedIn', me: await getMe() })
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) {
        console.error('session: cannot read the current session', err)
      }
      setState({ status: 'anonymous' })
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    getMe().then(
      (me) => {
        if (!cancelled) setState({ status: 'signedIn', me })
      },
      (err: unknown) => {
        if (cancelled) return
        if (!(err instanceof ApiError && err.status === 401)) {
          console.error('session: cannot read the current session', err)
        }
        setState({ status: 'anonymous' })
      },
    )
    return () => {
      cancelled = true
    }
  }, [])

  const setMe = useCallback((me: Me) => {
    setState({ status: 'signedIn', me })
  }, [])

  const logout = useCallback(async () => {
    try {
      await postLogout()
    } catch {
      // The local state is cleared either way.
    }
    setState({ status: 'anonymous' })
    navigate('/')
  }, [])

  const value = useMemo(() => ({ state, setMe, refresh, logout }), [state, setMe, refresh, logout])
  return <SessionContext value={value}>{children}</SessionContext>
}

// eslint-disable-next-line react-refresh/only-export-components -- the hook belongs to the provider
export function useSession(): SessionValue {
  const value = useContext(SessionContext)
  if (value === null) {
    throw new Error('useSession must be used inside SessionProvider')
  }
  return value
}

import { useMemo, useSyncExternalStore } from 'react'

export type Route =
  | { name: 'home' }
  | { name: 'setup' }
  | { name: 'login' }
  | { name: 'table' }
  | { name: 'play'; token: string }
  | { name: 'reset'; token: string }
  | { name: 'notFound' }

const navigateEvent = 'dndvtt:navigate'
const tokenPattern = /^[A-Za-z0-9_-]+$/

/** matchRoute maps a pathname to a route. Tokens are one path segment of [A-Za-z0-9_-]+. */
export function matchRoute(pathname: string): Route {
  // One trailing slash is allowed (/login/); "//" is not a route.
  const path = pathname.replace(/^(.*[^/])\/$/, '$1')
  if (path === '/') return { name: 'home' }
  if (path === '/setup') return { name: 'setup' }
  if (path === '/login') return { name: 'login' }
  if (path === '/table') return { name: 'table' }

  const [, first, token, ...rest] = path.split('/')
  if (token !== undefined && rest.length === 0 && tokenPattern.test(token)) {
    if (first === 'play') return { name: 'play', token }
    if (first === 'reset') return { name: 'reset', token }
  }
  return { name: 'notFound' }
}

/** navigate pushes a new history entry (replace: true replaces it) and notifies useRoute. */
export function navigate(to: string, opts?: { replace?: boolean }): void {
  if (opts?.replace === true) {
    history.replaceState(null, '', to)
  } else {
    history.pushState(null, '', to)
  }
  window.dispatchEvent(new Event(navigateEvent))
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('popstate', onChange)
  window.addEventListener(navigateEvent, onChange)
  return () => {
    window.removeEventListener('popstate', onChange)
    window.removeEventListener(navigateEvent, onChange)
  }
}

function currentPathname(): string {
  return window.location.pathname
}

/** useRoute returns the current route and re-renders on navigate and on browser back/forward. */
export function useRoute(): Route {
  const pathname = useSyncExternalStore(subscribe, currentPathname)
  return useMemo(() => matchRoute(pathname), [pathname])
}

import { useEffect, useState } from 'react'

const narrowQuery = '(max-width: 899px)'

/** useNarrow tells whether the screen is narrower than 900 px, where the sections replace the panel. */
export function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => window.matchMedia(narrowQuery).matches)
  useEffect(() => {
    const query = window.matchMedia(narrowQuery)
    const onChange = () => {
      setNarrow(query.matches)
    }
    query.addEventListener('change', onChange)
    onChange()
    return () => {
      query.removeEventListener('change', onChange)
    }
  }, [])
  return narrow
}

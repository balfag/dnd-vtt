import { useEffect, useState } from 'react'

/**
 * useRestoreFocus gives the focus back to what had it when the component was drawn, once the component
 * is gone: a dialog that the parent removes from the tree cannot do it itself.
 */
export function useRestoreFocus(): void {
  const [opener] = useState<Element | null>(() => document.activeElement)
  useEffect(
    () => () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    },
    [opener],
  )
}

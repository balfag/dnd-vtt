import { useEffect, useState } from 'react'
import { docOf, isHTMLElement } from '../../../shared/dom'

/**
 * useRestoreFocus gives the focus back to what had it when the component was drawn, once the component
 * is gone: a dialog that the parent removes from the tree cannot do it itself.
 * The opener is looked up in the global document; in a detached window the dialog restores the focus itself.
 */
export function useRestoreFocus(): void {
  const [opener] = useState<Element | null>(() => docOf(null).activeElement)
  useEffect(
    () => () => {
      if (isHTMLElement(opener) && opener.isConnected) opener.focus()
    },
    [opener],
  )
}

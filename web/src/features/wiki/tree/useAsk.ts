import { useState } from 'react'

export interface Asked<T> {
  value: T
  /** open turns false first, so the dialog closes before it is dropped. */
  open: boolean
}

/** useAsk holds what a confirmation dialog is about: ask(value) opens it, close() closes it. */
export function useAsk<T>() {
  const [asked, setAsked] = useState<Asked<T> | null>(null)
  return {
    asked,
    ask: (value: T) => {
      setAsked({ value, open: true })
    },
    close: () => {
      setAsked((a) => (a === null ? null : { ...a, open: false }))
    },
  }
}

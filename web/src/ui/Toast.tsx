import { useEffect } from 'react'
import './ui.css'

export interface ToastMessage {
  /** id differs for every message, so a new toast restarts the timer of the old one. */
  id: number
  text: string
}

const visibleMs = 3000

/** Toast confirms an action for three seconds; a new message replaces the old one. */
export function Toast({ toast, onDone }: { toast: ToastMessage | null; onDone: () => void }) {
  useEffect(() => {
    if (toast === null) return
    const timer = setTimeout(onDone, visibleMs)
    return () => {
      clearTimeout(timer)
    }
  }, [toast, onDone])

  // The region is always there: a live region announces text that appears in it.
  return (
    <div className="toast-region" role="status">
      {toast !== null && <p className="toast">{toast.text}</p>}
    </div>
  )
}

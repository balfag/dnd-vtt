import { useEffect, useState } from 'react'

const visibleForMs = 3000

/** Announcement is one thing the canvas says; id changes with every new one, so a repeat is read again. */
export interface Announcement {
  id: number
  text: string
  /** Also show a note on the screen for 3 s, not only to screen readers. */
  visible: boolean
}

/** TableAnnouncer reads the canvas announcements aloud (aria-live) and shows the important ones. */
export function TableAnnouncer({ announcement }: { announcement: Announcement | null }) {
  const [hiddenId, setHiddenId] = useState<number | null>(null) // the note whose 3 s are over

  useEffect(() => {
    if (announcement === null || !announcement.visible) return
    const timer = setTimeout(() => {
      setHiddenId(announcement.id)
    }, visibleForMs)
    return () => {
      clearTimeout(timer)
    }
  }, [announcement])

  return (
    <>
      <p className="sr-only" role="status" aria-live="polite">
        {announcement?.text ?? ''}
      </p>
      {announcement !== null && announcement.visible && hiddenId !== announcement.id && (
        <p className="connection-note connection-note--below" aria-hidden="true">
          {announcement.text}
        </p>
      )}
    </>
  )
}

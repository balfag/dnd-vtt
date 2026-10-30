import { lazy, Suspense, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { isElement } from '../../shared/dom'
import type { ChatMessage } from '../../shared/protocol'
import { useHandoutQueue } from './useHandoutQueue'
import './handout.css'

// The editor (TipTap and ProseMirror) is a chunk of its own.
const WikiEditor = lazy(() =>
  import('../wiki/editor/WikiEditor').then((m) => ({ default: m.WikiEditor })),
)

interface HandoutModalProps {
  message: ChatMessage
  onClose: () => void
}

/** HandoutModal is the window of a handout: the title, the text read-only, and "Close". */
export function HandoutModal({ message, onClose }: HandoutModalProps) {
  const { t } = useTranslation()
  const titleId = useId()
  const dialogRef = useRef<HTMLDialogElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const handout = message.handout
  const title = handout?.title ?? ''

  useEffect(() => {
    const dialog = dialogRef.current
    if (dialog === null || dialog.open) return
    dialog.showModal()
    closeRef.current?.focus()
  }, [])

  if (handout === undefined) return null
  return (
    <dialog
      ref={dialogRef}
      className="handout-modal"
      aria-labelledby={titleId}
      aria-modal="true"
      onCancel={(e) => {
        // Escape closes through the same path as the button.
        e.preventDefault()
        onClose()
      }}
    >
      <div className="handout-modal__body">
        <div className="handout-modal__head">
          <h2 className="handout-modal__title" id={titleId}>
            {title === '' ? t('handout.untitled') : title}
          </h2>
          <span className="handout-modal__from">
            {message.visibility === 'private'
              ? t('handout.modal.fromPrivate')
              : t('handout.modal.from')}
          </span>
        </div>
        <div
          className="handout-modal__content"
          onClick={(e) => {
            const img = isElement(e.target) ? e.target.closest('img[data-asset-id]') : null
            const id = img?.getAttribute('data-asset-id')
            if (id !== null && id !== undefined) {
              window.open(`/assets/${id}`, '_blank', 'noopener')
            }
          }}
        >
          <Suspense fallback={null}>
            <WikiEditor content={handout.content} editable={false} />
          </Suspense>
        </div>
        <div className="handout-modal__foot">
          <p className="handout-modal__stays">{t('handout.modal.stays')}</p>
          <button ref={closeRef} type="button" className="button button--primary" onClick={onClose}>
            {t('handout.modal.close')}
          </button>
        </div>
      </div>
    </dialog>
  )
}

/**
 * HandoutHost shows the first handout of the queue, one window at a time, and reads each new one
 * aloud when it arrives ("The GM showed you: ...").
 */
export function HandoutHost() {
  const { t } = useTranslation()
  const { current, queue, close } = useHandoutQueue()
  const announced = useRef(new Set<string>())
  const [said, setSaid] = useState('')

  useEffect(() => {
    for (const m of queue) {
      if (announced.current.has(m.id)) continue
      announced.current.add(m.id)
      const title = m.handout?.title ?? ''
      setSaid(t('handout.modal.announce', { title: title === '' ? t('handout.untitled') : title }))
    }
  }, [queue, t])

  return (
    <>
      <p className="sr-only" role="status" aria-live="polite">
        {said}
      </p>
      {current !== null && <HandoutModal key={current.id} message={current} onClose={close} />}
    </>
  )
}

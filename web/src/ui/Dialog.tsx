import { useEffect, useId, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { docOf, isHTMLElement } from '../shared/dom'
import './ui.css'
import { Button } from './Button'

interface DialogProps {
  title: string
  open: boolean
  onClose: () => void
  children: React.ReactNode
  /** actions are the buttons under the content, for example the confirmation. */
  actions?: React.ReactNode
  /** closeLabel names the button that closes the dialog without acting: "Close" or "Cancel". */
  closeLabel?: string
}

/**
 * Dialog is a modal dialog on <dialog>: the browser traps the focus and closes
 * it on Escape; the focus returns to where it was before the dialog opened.
 */
export function Dialog({ title, open, onClose, children, actions, closeLabel }: DialogProps) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDialogElement>(null)
  const opener = useRef<Element | null>(null)
  const titleId = useId()

  useEffect(() => {
    const dialog = ref.current
    if (dialog === null) return
    if (open && !dialog.open) {
      opener.current = docOf(dialog).activeElement
      dialog.showModal()
    } else if (!open && dialog.open) {
      dialog.close()
    }
  }, [open])

  function closed() {
    onClose()
    if (isHTMLElement(opener.current)) {
      opener.current.focus()
    }
    opener.current = null
  }

  return (
    <dialog ref={ref} className="dialog" aria-labelledby={titleId} onClose={closed}>
      {open && (
        <div className="dialog__body">
          <div className="dialog__head">
            <h2 className="dialog__title" id={titleId}>
              {title}
            </h2>
          </div>
          <div className="dialog__content">{children}</div>
          <div className="dialog__actions">
            {/* First in the order of the focus: the safe choice gets it. */}
            <Button variant="secondary" fullWidth={false} onClick={onClose}>
              {closeLabel ?? t('ui.dialog.close')}
            </Button>
            {actions}
          </div>
        </div>
      )}
    </dialog>
  )
}

import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import './ui.css'
import { Button } from './Button'

interface CopyFieldProps {
  id: string
  label: string
  value: string
  onCopied?: () => void
  /** primary makes the copy button the main action of the place. */
  primary?: boolean
}

/** CopyField shows a read-only value with a button that copies it. */
export function CopyField({ id, label, value, onCopied, primary = false }: CopyFieldProps) {
  const { t } = useTranslation()
  const input = useRef<HTMLInputElement>(null)
  const [manual, setManual] = useState(false)

  async function copy() {
    try {
      // In an insecure context (http:// not from localhost) there is no clipboard.
      await navigator.clipboard.writeText(value)
      setManual(false)
      onCopied?.()
    } catch {
      input.current?.focus()
      input.current?.select()
      setManual(true)
    }
  }

  return (
    <div className="copy-field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      <div className="copy-field__row">
        <input
          ref={input}
          className="field__input"
          id={id}
          readOnly
          value={value}
          aria-describedby={manual ? `${id}-manual` : undefined}
          onFocus={(e) => {
            e.currentTarget.select()
          }}
        />
        <Button
          variant={primary ? 'primary' : 'secondary'}
          fullWidth={false}
          onClick={() => {
            void copy()
          }}
        >
          {t('ui.copy.button')}
        </Button>
      </div>
      {manual && (
        <p id={`${id}-manual`} className="field__text">
          {t('ui.copy.manual')}
        </p>
      )}
    </div>
  )
}

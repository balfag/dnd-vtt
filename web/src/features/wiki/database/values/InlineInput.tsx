import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

interface InlineInputProps {
  type: 'text' | 'number' | 'date' | 'url'
  initial: string
  label: string
  maxLength?: number
  step?: string
  /** validate gives the i18n key of what is wrong with the text, or null. */
  validate?: (text: string) => string | null
  /** onCommit gets the text when it is valid and the edit ends by Enter or by leaving the field. */
  onCommit: (text: string) => void
  /** onCancel ends the edit with the old value; why is set when the text was refused. */
  onCancel: (why?: string) => void
}

/**
 * InlineInput edits a value in place. The text lives in this field until Enter or blur,
 * so typing never redraws the table; Escape drops it.
 */
export function InlineInput(p: InlineInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState(p.initial)
  const [problem, setProblem] = useState<string | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const done = useRef(false)

  useEffect(() => {
    input.current?.focus()
    input.current?.select()
  }, [])

  function finish(commit: boolean, leaving: boolean) {
    if (done.current) return
    if (!commit) {
      done.current = true
      p.onCancel()
      return
    }
    const bad = input.current?.validity.badInput === true
    const why = bad ? 'wiki.db.value.notNumber' : (p.validate?.(text) ?? null)
    if (why !== null) {
      if (leaving) {
        done.current = true
        p.onCancel(why)
      } else {
        setProblem(why)
      }
      return
    }
    done.current = true
    p.onCommit(text)
  }

  return (
    <>
      <input
        ref={input}
        className="dbv__input"
        type={p.type === 'url' ? 'text' : p.type}
        step={p.step}
        inputMode={p.type === 'number' ? 'decimal' : undefined}
        aria-label={p.label}
        aria-invalid={problem !== null}
        maxLength={p.maxLength}
        value={text}
        onChange={(e) => {
          setText(e.target.value)
          setProblem(null)
        }}
        onKeyDown={(e) => {
          // The grid around must not read the arrows and Enter of a field being edited.
          e.stopPropagation()
          if (e.key === 'Enter') {
            e.preventDefault()
            finish(true, false)
          } else if (e.key === 'Escape') {
            e.preventDefault()
            finish(false, false)
          }
        }}
        onBlur={() => {
          finish(true, true)
        }}
      />
      {problem !== null && <p className="dbv__error">{t(problem)}</p>}
    </>
  )
}

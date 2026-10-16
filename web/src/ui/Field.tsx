interface FieldProps {
  id: string
  label: string
  type?: 'text' | 'password'
  value: string
  onChange: (value: string) => void
  error?: string
  hint?: string
  autoComplete: string
}

/** Field is a labelled text input with an optional hint and error. */
export function Field({
  id,
  label,
  type = 'text',
  value,
  onChange,
  error,
  hint,
  autoComplete,
}: FieldProps) {
  const noteId = `${id}-note`
  const note = error ?? hint
  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      <input
        className="field__input"
        id={id}
        type={type}
        value={value}
        autoComplete={autoComplete}
        aria-invalid={error !== undefined}
        aria-describedby={note === undefined ? undefined : noteId}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
      {note !== undefined && (
        <p
          id={noteId}
          className={error === undefined ? 'field__text' : 'field__text field__text--error'}
        >
          {note}
        </p>
      )}
    </div>
  )
}

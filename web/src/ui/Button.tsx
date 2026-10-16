import './ui.css'

interface ButtonProps {
  children: React.ReactNode
  type?: 'button' | 'submit'
  disabled?: boolean
  onClick?: () => void
  /** primary is the main action of a place; secondary and danger are quieter. */
  variant?: 'primary' | 'secondary' | 'danger'
  /** fullWidth (the default) fills the width of the column; false fits the text. */
  fullWidth?: boolean
  ref?: React.Ref<HTMLButtonElement>
}

export function Button({
  children,
  type = 'button',
  disabled = false,
  onClick,
  variant = 'primary',
  fullWidth = true,
  ref,
}: ButtonProps) {
  const className = ['button', `button--${variant}`, fullWidth ? '' : 'button--inline']
    .filter(Boolean)
    .join(' ')
  return (
    <button ref={ref} className={className} type={type} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  )
}

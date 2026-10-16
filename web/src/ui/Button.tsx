interface ButtonProps {
  children: React.ReactNode
  type?: 'button' | 'submit'
  disabled?: boolean
  onClick?: () => void
}

/** Button is the primary action of a screen. */
export function Button({ children, type = 'button', disabled = false, onClick }: ButtonProps) {
  return (
    <button className="button" type={type} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  )
}

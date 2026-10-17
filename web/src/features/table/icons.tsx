// Tool icons: one line of 1.6px, drawn in currentColor, no icon library.

function Icon({ children }: { children: React.ReactNode }) {
  return (
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  )
}

/** SelectIcon is an arrow: select and move tokens. */
export function SelectIcon() {
  return (
    <Icon>
      <path d="M6 4l12 7.2-5.2 1.4 3 5.4-2.4 1.3-3-5.4L6 18z" />
    </Icon>
  )
}
export function PlusIcon() {
  return (
    <Icon>
      <path d="M12 5v14M5 12h14" />
    </Icon>
  )
}

export function MinusIcon() {
  return (
    <Icon>
      <path d="M5 12h14" />
    </Icon>
  )
}

/** FitIcon is four corners: the whole scene in view. */
export function FitIcon() {
  return (
    <Icon>
      <path d="M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5" />
    </Icon>
  )
}

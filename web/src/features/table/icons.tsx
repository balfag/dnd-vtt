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

/** PlaceTokenIcon is a token ring with a plus: put a new token on the map. */
export function PlaceTokenIcon() {
  return (
    <Icon>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 8.5v7M8.5 12h7" />
    </Icon>
  )
}

/** RevealFogIcon is a dashed frame with a clear circle: open a place in the fog. */
export function RevealFogIcon() {
  return (
    <Icon>
      <rect x="4" y="4" width="16" height="16" rx="2" strokeDasharray="3 3" />
      <circle cx="12" cy="12" r="4" />
    </Icon>
  )
}

/** HideFogIcon is a cloud: cover a place with fog. */
export function HideFogIcon() {
  return (
    <Icon>
      <path d="M7 18h10.2a3.8 3.8 0 0 0 .5-7.57A5.5 5.5 0 0 0 7.1 9.4 4.3 4.3 0 0 0 7 18z" />
    </Icon>
  )
}
/** RulerIcon is a slanted ruler with ticks: measure a distance. */
export function RulerIcon() {
  return (
    <Icon>
      <path d="M4.5 15.5l11-11 4 4-11 11z" />
      <path d="M8 12l2 2M11 9l1.5 1.5M14 6l2 2" />
    </Icon>
  )
}

/** PingIcon is a dot with a ring: mark a place for everyone. */
export function PingIcon() {
  return (
    <Icon>
      <circle cx="12" cy="12" r="2" />
      <circle cx="12" cy="12" r="6.5" />
    </Icon>
  )
}

/** DrawIcon is a pencil: draw on the map. */
export function DrawIcon() {
  return (
    <Icon>
      <path d="M4 20l1-4.5L16.5 4a2 2 0 0 1 3 3L8 18.5z" />
      <path d="M14.5 6l3 3" />
    </Icon>
  )
}

/** TemplateIcon is a cone: an area of effect. */
export function TemplateIcon() {
  return (
    <Icon>
      <path d="M4 12L20 5v14z" />
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

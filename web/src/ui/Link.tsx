import { navigate } from '../app/router'

/** Link is an in-app link: it navigates without a page reload. */
export function Link({ to, children }: { to: string; children: React.ReactNode }) {
  return (
    <a
      className="link"
      href={to}
      onClick={(e) => {
        e.preventDefault()
        navigate(to)
      }}
    >
      {children}
    </a>
  )
}

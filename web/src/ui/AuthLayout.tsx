import './ui.css'
import { FogMap } from './FogMap'

interface AuthLayoutProps {
  /** A line above the title in plain case, for example the campaign name. */
  context?: string
  title: string
  children: React.ReactNode
}

/** AuthLayout is a left-aligned column of a sign-in screen with the fog map on the right. */
export function AuthLayout({ context, title, children }: AuthLayoutProps) {
  return (
    <main className="auth">
      <div className="auth__column">
        {context !== undefined && <p className="auth__context">{context}</p>}
        <h1 className="auth__title">{title}</h1>
        {children}
      </div>
      <FogMap />
    </main>
  )
}

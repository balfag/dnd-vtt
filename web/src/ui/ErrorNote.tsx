/** ErrorNote tells what went wrong and what to do. */
export function ErrorNote({ children }: { children: React.ReactNode }) {
  return (
    <p className="error-note" role="alert">
      {children}
    </p>
  )
}

// The rules match the server (docs/architecture/security.md); the server still checks everything itself.

/** normalizeLogin trims the login and lowercases it, as the server does. */
export function normalizeLogin(login: string): string {
  return login.trim().toLowerCase()
}

export function isValidLogin(login: string): boolean {
  return /^[a-z0-9_.-]{3,32}$/.test(normalizeLogin(login))
}

/** Length is counted in code points, like the server counts runes. */
function length(s: string): number {
  return Array.from(s).length
}

export function isValidPassword(password: string): boolean {
  const n = length(password)
  return n >= 8 && n <= 128
}

export function isValidDisplayName(name: string): boolean {
  const n = length(name.trim())
  return n >= 1 && n <= 64
}

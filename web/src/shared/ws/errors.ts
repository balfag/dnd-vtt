import { errorCodes, errorKey } from '../api/errors'
import type { ErrorCode } from '../protocol'

/** CommandError is a failed command: a server error code, or 'timeout' / 'disconnected'. */
export class CommandError extends Error {
  readonly code: ErrorCode | 'timeout' | 'disconnected'

  constructor(code: ErrorCode | 'timeout' | 'disconnected', message?: string) {
    super(message ?? code)
    this.name = 'CommandError'
    this.code = code
  }
}

/** commandErrorKey returns the i18n key that explains a failed command to the user. */
export function commandErrorKey(err: unknown): string {
  if (err instanceof CommandError) {
    const known = errorCodes.find((code) => code === err.code)
    if (known !== undefined) return `errors.${known}`
    if (err.code === 'disconnected' || err.code === 'timeout') return 'errors.network'
  }
  return errorKey(err)
}

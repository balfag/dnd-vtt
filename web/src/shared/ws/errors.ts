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

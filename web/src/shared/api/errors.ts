import { ApiError } from './client'

/** errorCodes are the API error codes that have their own text. */
export const errorCodes = [
  'bad_request',
  'invalid_credentials',
  'unauthorized',
  'forbidden',
  'no_active_game',
  'removed',
  'not_found',
  'conflict',
  'rate_limited',
  'internal',
  'network',
] as const

/** errorKey returns the i18n key that explains err to the user. */
export function errorKey(err: unknown): string {
  if (err instanceof ApiError) {
    const known = errorCodes.find((code) => code === err.code)
    if (known !== undefined) {
      return `errors.${known}`
    }
  }
  return 'errors.unknown'
}

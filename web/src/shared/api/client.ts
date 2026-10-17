/** ApiError is any failure of an API call. */
export class ApiError extends Error {
  /** HTTP status; 0 for a network failure. */
  readonly status: number
  /** error.code from the body; 'network' or 'unknown' when absent. */
  readonly code: string

  constructor(status: number, code: string) {
    super(`api error ${String(status)} ${code}`)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

/** errorCodeOf reads error.code of an API error body. */
export function errorCodeOf(body: unknown): string {
  if (typeof body === 'object' && body !== null && 'error' in body) {
    const err = body.error
    if (typeof err === 'object' && err !== null && 'code' in err && typeof err.code === 'string') {
      return err.code
    }
  }
  return 'unknown'
}

async function readJSON(res: Response): Promise<unknown> {
  try {
    return await res.json()
  } catch {
    return undefined
  }
}

/**
 * apiRequest sends JSON (when body is given), parses a JSON reply with guard,
 * returns undefined for 204 or when no guard is given, and throws ApiError on any failure.
 */
export async function apiRequest<T>(
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  path: string,
  opts?: { body?: unknown; guard?: (v: unknown) => v is T },
): Promise<T | undefined> {
  const hasBody = opts?.body !== undefined
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: hasBody ? { 'Content-Type': 'application/json' } : undefined,
      body: hasBody ? JSON.stringify(opts.body) : undefined,
    })
  } catch {
    throw new ApiError(0, 'network')
  }

  if (!res.ok) {
    throw new ApiError(res.status, errorCodeOf(await readJSON(res)))
  }
  const guard = opts?.guard
  if (res.status === 204 || guard === undefined) {
    return undefined
  }
  const body = await readJSON(res)
  if (!guard(body)) {
    throw new ApiError(res.status, 'unknown')
  }
  return body
}

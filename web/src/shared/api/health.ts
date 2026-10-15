/** Health is the response of GET /api/health. */
export interface Health {
  status: 'ok'
  version: string
}

function isHealth(value: unknown): value is Health {
  return (
    typeof value === 'object' &&
    value !== null &&
    'status' in value &&
    value.status === 'ok' &&
    'version' in value &&
    typeof value.version === 'string'
  )
}

/** fetchHealth asks the server whether it is up and which version it runs. */
export async function fetchHealth(signal?: AbortSignal): Promise<Health> {
  const res = await fetch('/api/health', { signal })
  if (!res.ok) {
    throw new Error(`health: unexpected status ${String(res.status)}`)
  }
  const body: unknown = await res.json()
  if (!isHealth(body)) {
    throw new Error('health: malformed response')
  }
  return body
}

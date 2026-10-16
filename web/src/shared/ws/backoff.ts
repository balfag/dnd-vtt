const baseMs = 500
const maxMs = 10000

/**
 * reconnectDelay returns the pause before reconnect attempt n (1-based), in ms:
 * min(10000, 500 * 2^(n-1)) scaled by a factor in [0.8, 1.2] taken from random() in [0, 1).
 */
export function reconnectDelay(attempt: number, random: () => number): number {
  const n = Math.max(1, attempt)
  const base = Math.min(maxMs, baseMs * 2 ** (n - 1))
  return Math.round(base * (0.8 + 0.4 * random()))
}

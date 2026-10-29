import type { Point } from './camera'

function distanceToSegment(p: Point, a: Point, b: Point): number {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len2 = dx * dx + dy * dy
  if (len2 === 0) return Math.hypot(p.x - a.x, p.y - a.y)
  const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2))
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy))
}

/**
 * simplify drops the points of a polyline that lie within epsilon of the line through their
 * neighbours (Ramer–Douglas–Peucker). The ends stay; epsilon <= 0 changes nothing.
 */
export function simplify(points: Point[], epsilon: number): Point[] {
  if (epsilon <= 0 || points.length < 3) return [...points]
  const keep = new Array<boolean>(points.length).fill(false)
  keep[0] = true
  keep[points.length - 1] = true
  // An explicit stack: a long stroke must not overflow the call stack.
  const stack: [number, number][] = [[0, points.length - 1]]
  for (let range = stack.pop(); range !== undefined; range = stack.pop()) {
    const [first, last] = range
    const a = points[first]
    const b = points[last]
    if (a === undefined || b === undefined) continue
    let worst = -1
    let far = 0
    for (let i = first + 1; i < last; i++) {
      const p = points[i]
      if (p === undefined) continue
      const d = distanceToSegment(p, a, b)
      if (d > far) {
        far = d
        worst = i
      }
    }
    if (worst >= 0 && far > epsilon) {
      keep[worst] = true
      stack.push([first, worst], [worst, last])
    }
  }
  return points.filter((_, i) => keep[i] === true)
}

/** thin keeps at most max points, evenly spread, with both ends. */
export function thin(points: Point[], max: number): Point[] {
  if (points.length <= max || max < 2) return [...points]
  const out: Point[] = []
  for (let i = 0; i < max; i++) {
    const p = points[Math.round((i * (points.length - 1)) / (max - 1))]
    if (p !== undefined) out.push(p)
  }
  return out
}

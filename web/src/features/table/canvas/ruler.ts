import type { Grid } from '../../../shared/protocol'
import type { Point } from './camera'
import { cellAt, cellCenter, cellDistanceFt } from './grid'

/** rulerPath snaps every waypoint to a cell centre (g) and returns the polyline. */
export function rulerPath(g: Grid, waypoints: Point[]): Point[] {
  return waypoints.map((p) => cellCenter(g, cellAt(g, p)))
}

/** rulerDistanceFt sums cellDistanceFt over the segments. */
export function rulerDistanceFt(g: Grid, path: Point[]): number {
  let total = 0
  for (let i = 1; i < path.length; i++) {
    const a = path[i - 1]
    const b = path[i]
    if (a !== undefined && b !== undefined) total += cellDistanceFt(g, a, b)
  }
  return total
}

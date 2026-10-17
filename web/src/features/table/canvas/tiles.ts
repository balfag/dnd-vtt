export type TileRect = { x: number; y: number; w: number; h: number }

/** tileRects splits an image into tiles no larger than max on each side, row by row. */
export function tileRects(width: number, height: number, max: number): TileRect[] {
  const tiles: TileRect[] = []
  for (let y = 0; y < height; y += max) {
    for (let x = 0; x < width; x += max) {
      tiles.push({ x, y, w: Math.min(max, width - x), h: Math.min(max, height - y) })
    }
  }
  return tiles
}

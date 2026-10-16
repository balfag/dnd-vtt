const ROOM =
  'M640 210 L880 190 L970 260 L950 430 L870 450 L865 560 L800 600 L730 560 L700 450 L630 400 Z'

// Pointy-top hexagons of side 26 on a 45 x 78 tile (the tile repeats every two rows).
const HEX_W = 45
const HEX_H = 78
const HEX_SIDE = 26

function hexPath(cx: number, cy: number): string {
  const half = HEX_W / 2
  const quarter = HEX_SIDE / 2
  const points = [
    [cx, cy - HEX_SIDE],
    [cx + half, cy - quarter],
    [cx + half, cy + quarter],
    [cx, cy + HEX_SIDE],
    [cx - half, cy + quarter],
    [cx - half, cy - quarter],
  ]
  return `M${points.map(([x, y]) => `${String(x)} ${String(y)}`).join(' L')} Z`
}

const HEX_TILE = [
  hexPath(0, 0),
  hexPath(HEX_W, 0),
  hexPath(HEX_W / 2, HEX_H / 2),
  hexPath(0, HEX_H),
  hexPath(HEX_W, HEX_H),
].join(' ')

const TOKENS = [
  { x: 730, y: 300, color: '#4FA58E' },
  { x: 820, y: 360, color: '#1e88e5' },
  { x: 890, y: 270, color: '#C9A34E' },
]

/** FogMap is the room opened out of the fog: the one memorable detail of the sign-in screens. */
export function FogMap() {
  return (
    <div className="fogmap" aria-hidden="true">
      <svg viewBox="0 0 1200 800" preserveAspectRatio="xMidYMid slice">
        <defs>
          <clipPath id="fogmap-room">
            <path d={ROOM} />
          </clipPath>
          <pattern id="fogmap-hex" width={HEX_W} height={HEX_H} patternUnits="userSpaceOnUse">
            <path className="fogmap__hex" d={HEX_TILE} />
          </pattern>
        </defs>
        <g clipPath="url(#fogmap-room)">
          <rect className="fogmap__room" x="600" y="150" width="400" height="500" />
          <rect x="600" y="150" width="400" height="500" fill="url(#fogmap-hex)" />
        </g>
        <path className="fogmap__outline" d={ROOM} />
        {TOKENS.map((token) => (
          <circle
            key={token.color}
            className="fogmap__token"
            cx={token.x}
            cy={token.y}
            r="15"
            fill={token.color}
          />
        ))}
      </svg>
      <div className="fog" />
    </div>
  )
}

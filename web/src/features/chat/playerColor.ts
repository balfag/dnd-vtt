const chalk = '#ece7db'
const slate = '#23303a'
const minContrast = 4.5

function channels(hex: string): [number, number, number] {
  const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex)
  if (m === null) return [0, 0, 0]
  return [parseInt(m[1] ?? '0', 16), parseInt(m[2] ?? '0', 16), parseInt(m[3] ?? '0', 16)]
}

function luminance(hex: string): number {
  const [r, g, b] = channels(hex).map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  })
  return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (b ?? 0)
}

/** contrast is the WCAG 2.x contrast ratio of two #rrggbb colors. */
export function contrast(a: string, b: string): number {
  const la = luminance(a)
  const lb = luminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

function mix(from: string, to: string, share: number): string {
  const f = channels(from)
  const t = channels(to)
  return (
    '#' +
    f
      .map((v, i) => {
        const w = t[i] ?? 0
        return Math.round(v + (w - v) * share)
          .toString(16)
          .padStart(2, '0')
      })
      .join('')
  )
}

/**
 * readableColor mixes hex toward --chalk (#ECE7DB) in 10% steps until its WCAG contrast
 * against --slate (#23303A) is at least 4.5; returns lowercase #rrggbb.
 */
export function readableColor(hex: string): string {
  const start = hex.toLowerCase()
  for (let step = 0; step <= 10; step++) {
    const color = mix(start, chalk, step / 10)
    if (contrast(color, slate) >= minContrast) return color
  }
  return chalk
}

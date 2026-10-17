/** The server accepts an initiative from -99 to 99. */
export const initiativeLimits = { min: -99, max: 99 }

/**
 * parseInitiative reads what was typed into the initiative field: a whole number within the
 * limits, null for an empty field (clears the initiative), undefined for anything else.
 */
export function parseInitiative(text: string): number | null | undefined {
  const s = text.trim()
  if (s === '') return null
  if (!/^[-−]?\d{1,2}$/.test(s)) return undefined
  const n = Number(s.replace('−', '-')) + 0 // + 0: "-0" is 0
  return n >= initiativeLimits.min && n <= initiativeLimits.max ? n : undefined
}

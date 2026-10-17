/** CONDITIONS are the keys of the conditions of SRD 5.2; the names are `conditions.<key>` in the translations. */
export const CONDITIONS = [
  'blinded',
  'charmed',
  'deafened',
  'exhaustion',
  'frightened',
  'grappled',
  'incapacitated',
  'invisible',
  'paralyzed',
  'petrified',
  'poisoned',
  'prone',
  'restrained',
  'stunned',
  'unconscious',
] as const

export type Condition = (typeof CONDITIONS)[number]

export function isCondition(key: string): key is Condition {
  return CONDITIONS.some((c) => c === key)
}

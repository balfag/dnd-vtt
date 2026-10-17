import type { Combat, Token } from '../../shared/protocol'
import type { Me } from './combatRights'

/** TurnChange is the turn that has just begun, for the announcement. */
export interface TurnChange {
  /** Empty when the viewer may not see the name. */
  name: string
  /** The viewer owns the token of the combatant. */
  mine: boolean
}

/**
 * turnChange tells which turn began between two states of the same fight, or null when the turn
 * did not move: a fight that has just appeared (a start, a reconnect) is not announced.
 */
export function turnChange(
  prev: Combat | null,
  next: Combat | null,
  tokens: Token[],
  me: Me,
): TurnChange | null {
  if (prev === null || next === null || prev.sceneId !== next.sceneId) return null
  if (next.turnCombatantId === null) return null
  if (prev.turnCombatantId === next.turnCombatantId && prev.round === next.round) return null
  const current = next.combatants.find((c) => c.id === next.turnCombatantId)
  if (current === undefined) return null
  const owners = tokens.find((t) => t.id === current.tokenId)?.owners ?? []
  return {
    name: current.name,
    mine: me.playerId !== null && owners.includes(me.playerId),
  }
}

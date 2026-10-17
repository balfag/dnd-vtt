import type { Combat, Combatant, Token } from '../../shared/protocol'

/** Me is who looks at the fight: the GM, or a player (null id: not in the game). */
export interface Me {
  isGM: boolean
  playerId: string | null
}

function ownsToken(tokenId: string, tokens: Token[], me: Me): boolean {
  if (me.playerId === null) return false
  return tokens.find((t) => t.id === tokenId)?.owners.includes(me.playerId) ?? false
}

/** canSetInitiative: the GM, or the owner of the token of the combatant. */
export function canSetInitiative(c: Combatant, tokens: Token[], me: Me): boolean {
  return me.isGM || ownsToken(c.tokenId, tokens, me)
}

/** canEndTurn: the GM, or the owner of the token of the combatant whose turn it is. */
export function canEndTurn(combat: Combat, tokens: Token[], me: Me): boolean {
  if (me.isGM) return true
  const current = combat.combatants.find((c) => c.id === combat.turnCombatantId)
  return current !== undefined && ownsToken(current.tokenId, tokens, me)
}

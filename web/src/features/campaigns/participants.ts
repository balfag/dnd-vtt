import type { CampaignPlayer } from '../../shared/api/campaigns'
import type { PlayerInfo } from '../../shared/protocol'

/**
 * mergeParticipants lists the participants of the REST answer in its order,
 * without the removed ones. The online flag comes from live (the game store)
 * when it knows the participant, otherwise from the REST answer.
 */
export function mergeParticipants(
  rest: CampaignPlayer[],
  live: PlayerInfo[] | null,
): CampaignPlayer[] {
  const online = new Map<string, boolean>()
  for (const p of live ?? []) online.set(p.id, p.online)

  return rest
    .filter((p) => !p.removed)
    .map((p) => {
      const now = online.get(p.id)
      return now === undefined ? p : { ...p, online: now }
    })
}

/** unknownParticipantIds lists who live has and the REST list lacks (a late registration). */
export function unknownParticipantIds(rest: CampaignPlayer[], live: PlayerInfo[]): string[] {
  return live.filter((p) => !rest.some((r) => r.id === p.id)).map((p) => p.id)
}

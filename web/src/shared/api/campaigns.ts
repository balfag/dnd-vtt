import { ApiError, apiRequest } from './client'

export interface GameSessionRef {
  id: string
  startedAt: number
}

export interface Campaign {
  id: string
  name: string
  createdAt: number
  activeGameSession: GameSessionRef | null
}

export interface StartedGame {
  id: string
  campaignId: string
  startedAt: number
  link: string
}

export interface CampaignPlayer {
  id: string
  accountId: string
  accountLogin: string
  displayName: string
  color: string
  role: 'gm' | 'player'
  removed: boolean
  online: boolean
}

export interface PasswordResetLink {
  link: string
  expiresAt: number
}

const colorPattern = /^#[0-9a-f]{6}$/

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isGameSessionRef(v: unknown): v is GameSessionRef {
  return isRecord(v) && typeof v.id === 'string' && typeof v.startedAt === 'number'
}

export function isCampaign(v: unknown): v is Campaign {
  return (
    isRecord(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    typeof v.createdAt === 'number' &&
    // The field must be present: null means "no game", a missing one is a broken reply.
    'activeGameSession' in v &&
    (v.activeGameSession === null || isGameSessionRef(v.activeGameSession))
  )
}

function isCampaignList(v: unknown): v is Campaign[] {
  return Array.isArray(v) && v.every(isCampaign)
}

function isStartedGame(v: unknown): v is StartedGame {
  return (
    isRecord(v) &&
    typeof v.id === 'string' &&
    typeof v.campaignId === 'string' &&
    typeof v.startedAt === 'number' &&
    typeof v.link === 'string'
  )
}

export function isCampaignPlayer(v: unknown): v is CampaignPlayer {
  return (
    isRecord(v) &&
    typeof v.id === 'string' &&
    typeof v.accountId === 'string' &&
    typeof v.accountLogin === 'string' &&
    typeof v.displayName === 'string' &&
    typeof v.color === 'string' &&
    colorPattern.test(v.color) &&
    (v.role === 'gm' || v.role === 'player') &&
    typeof v.removed === 'boolean' &&
    typeof v.online === 'boolean'
  )
}

function isPlayerList(v: unknown): v is CampaignPlayer[] {
  return Array.isArray(v) && v.every(isCampaignPlayer)
}

function isLinkReply(v: unknown): v is { link: string } {
  return isRecord(v) && typeof v.link === 'string'
}

function isPasswordResetLink(v: unknown): v is PasswordResetLink {
  return isRecord(v) && typeof v.link === 'string' && typeof v.expiresAt === 'number'
}

/** present rejects a reply that should have had a body but did not. */
function present<T>(value: T | undefined): T {
  if (value === undefined) {
    throw new ApiError(200, 'unknown')
  }
  return value
}

const enc = encodeURIComponent

export async function listCampaigns(): Promise<Campaign[]> {
  return present(await apiRequest('GET', '/api/campaigns', { guard: isCampaignList }))
}

export async function createCampaign(name: string): Promise<Campaign> {
  return present(await apiRequest('POST', '/api/campaigns', { body: { name }, guard: isCampaign }))
}

export async function renameCampaign(id: string, name: string): Promise<Campaign> {
  return present(
    await apiRequest('PATCH', `/api/campaigns/${enc(id)}`, { body: { name }, guard: isCampaign }),
  )
}

/** startGame starts a game; the reply holds the only copy of the game link the server gives. */
export async function startGame(campaignId: string): Promise<StartedGame> {
  return present(
    await apiRequest('POST', `/api/campaigns/${enc(campaignId)}/game-sessions`, {
      guard: isStartedGame,
    }),
  )
}

export async function endGame(gameSessionId: string): Promise<void> {
  await apiRequest('POST', `/api/game-sessions/${enc(gameSessionId)}/end`)
}

export async function rotateLink(gameSessionId: string): Promise<{ link: string }> {
  return present(
    await apiRequest('POST', `/api/game-sessions/${enc(gameSessionId)}/link`, {
      guard: isLinkReply,
    }),
  )
}

export async function listPlayers(campaignId: string): Promise<CampaignPlayer[]> {
  return present(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/players`, { guard: isPlayerList }),
  )
}

export async function removePlayer(playerId: string): Promise<void> {
  await apiRequest('POST', `/api/players/${enc(playerId)}/remove`)
}

export async function createPasswordReset(accountId: string): Promise<PasswordResetLink> {
  return present(
    await apiRequest('POST', `/api/accounts/${enc(accountId)}/password-resets`, {
      guard: isPasswordResetLink,
    }),
  )
}

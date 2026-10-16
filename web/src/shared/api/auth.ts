import { ApiError, apiRequest } from './client'

export interface MePlayer {
  id: string
  campaignId: string
  displayName: string
  color: string
  role: 'gm' | 'player'
}

export interface Me {
  accountId: string
  login: string
  isGm: boolean
  player: MePlayer | null
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

function isMePlayer(v: unknown): v is MePlayer {
  return (
    isRecord(v) &&
    typeof v.id === 'string' &&
    typeof v.campaignId === 'string' &&
    typeof v.displayName === 'string' &&
    typeof v.color === 'string' &&
    (v.role === 'gm' || v.role === 'player')
  )
}

export function isMe(v: unknown): v is Me {
  return (
    isRecord(v) &&
    typeof v.accountId === 'string' &&
    typeof v.login === 'string' &&
    typeof v.isGm === 'boolean' &&
    (v.player === null || isMePlayer(v.player))
  )
}

function isSetupState(v: unknown): v is { needed: boolean } {
  return isRecord(v) && typeof v.needed === 'boolean'
}

function isGameLink(v: unknown): v is { campaignName: string } {
  return isRecord(v) && typeof v.campaignName === 'string'
}

/** present rejects a reply that should have had a body but did not. */
function present<T>(value: T | undefined): T {
  if (value === undefined) {
    throw new ApiError(200, 'unknown')
  }
  return value
}

/** getMe asks who is signed in; it fails with status 401 when nobody is. */
export async function getMe(): Promise<Me> {
  return present(await apiRequest<Me>('GET', '/api/auth/me', { guard: isMe }))
}

export async function getSetupNeeded(): Promise<boolean> {
  return present(await apiRequest('GET', '/api/setup', { guard: isSetupState })).needed
}

export async function postSetup(setupToken: string, login: string, password: string): Promise<Me> {
  return present(
    await apiRequest<Me>('POST', '/api/setup', {
      body: { setupToken, login, password },
      guard: isMe,
    }),
  )
}

export async function postGMToken(token: string): Promise<Me> {
  return present(
    await apiRequest<Me>('POST', '/api/auth/gm-token', { body: { token }, guard: isMe }),
  )
}

/** postLogin signs in; gameLink is the token of a game link, for players. */
export async function postLogin(login: string, password: string, gameLink?: string): Promise<Me> {
  return present(
    await apiRequest<Me>('POST', '/api/auth/login', {
      body: { login, password, gameLink },
      guard: isMe,
    }),
  )
}

export async function postRegister(
  gameLink: string,
  login: string,
  password: string,
  displayName: string,
): Promise<Me> {
  return present(
    await apiRequest<Me>('POST', '/api/auth/register', {
      body: { gameLink, login, password, displayName },
      guard: isMe,
    }),
  )
}

export async function postLogout(): Promise<void> {
  await apiRequest('POST', '/api/auth/logout')
}

export async function getGameLink(token: string): Promise<{ campaignName: string }> {
  return present(
    await apiRequest('GET', `/api/game-links/${encodeURIComponent(token)}`, { guard: isGameLink }),
  )
}

export async function postPasswordReset(token: string, newPassword: string): Promise<void> {
  await apiRequest('POST', `/api/password-resets/${encodeURIComponent(token)}`, {
    body: { new: newPassword },
  })
}

import { randomBytes } from 'node:crypto'
import { expect, test } from '@playwright/test'
import ru from '../src/i18n/ru.json' with { type: 'json' }

function password(): string {
  return randomBytes(8).toString('hex')
}

test('gm sets up, a player registers by the game link and the link can be replaced', async ({
  browser,
}) => {
  const setupURL = process.env.DNDVTT_E2E_SETUP_URL
  if (setupURL === undefined) throw new Error('DNDVTT_E2E_SETUP_URL is not set')
  const gmPassword = password()
  const playerPassword = password()
  const gmLogin = 'master'
  const playerName = 'Анна'

  // 1. The GM creates the account on the first run.
  const gmContext = await browser.newContext()
  const gm = await gmContext.newPage()
  await gm.goto(setupURL)
  await gm.getByLabel(ru.common.login, { exact: true }).fill(gmLogin)
  await gm.getByLabel(ru.common.password, { exact: true }).fill(gmPassword)
  await gm.getByLabel(ru.common.passwordRepeat, { exact: true }).fill(gmPassword)
  await gm.getByRole('button', { name: ru.setup.submit }).click()
  await expect(gm.getByRole('heading', { name: ru.campaigns.title })).toBeVisible()

  // 2. The GM starts a game over REST.
  const campaignRes = await gmContext.request.post('/api/campaigns', { data: { name: 'E2E' } })
  expect(campaignRes.status()).toBe(201)
  const campaign = (await campaignRes.json()) as { id: string }
  const sessionRes = await gmContext.request.post(`/api/campaigns/${campaign.id}/game-sessions`)
  expect(sessionRes.status()).toBe(201)
  const gameSession = (await sessionRes.json()) as { id: string; link: string }
  const oldLink = gameSession.link

  // 3. The player registers by the game link.
  const playerContext = await browser.newContext()
  const player = await playerContext.newPage()
  await player.goto(oldLink)
  await expect(player.getByText('E2E', { exact: true })).toBeVisible()
  await player.getByRole('tab', { name: ru.play.tabRegister }).click()
  await player.getByLabel(ru.common.login, { exact: true }).fill('anna')
  await player.getByLabel(ru.common.displayName, { exact: true }).fill(playerName)
  await player.getByLabel(ru.common.password, { exact: true }).fill(playerPassword)
  await player.getByLabel(ru.common.passwordRepeat, { exact: true }).fill(playerPassword)
  await player.getByRole('button', { name: ru.play.submitRegister }).click()
  await expect(player).toHaveURL(/\/table$/)
  await expect(player.getByText(ru.table.empty.player)).toBeVisible()

  // 4. The GM sees the player in the campaign.
  const playersRes = await gmContext.request.get(`/api/campaigns/${campaign.id}/players`)
  expect(playersRes.ok()).toBe(true)
  const players = (await playersRes.json()) as { displayName: string; role: string }[]
  expect(players).toContainEqual(
    expect.objectContaining({ displayName: playerName, role: 'player' }),
  )

  // 5. After the GM replaces the link, the old one is dead.
  const relinkRes = await gmContext.request.post(`/api/game-sessions/${gameSession.id}/link`)
  expect(relinkRes.ok()).toBe(true)
  const stranger = await (await browser.newContext()).newPage()
  await stranger.goto(oldLink)
  await expect(stranger.getByRole('heading', { name: ru.play.linkDeadTitle })).toBeVisible()

  await gmContext.close()
  await playerContext.close()
  await stranger.context().close()
})

import ru from '../src/i18n/ru.json' with { type: 'json' }
import { expect, test, registerPlayer, setupGM } from './helpers'

test('gm sets up, a player registers by the game link and the link can be replaced', async ({
  browser,
}) => {
  const playerName = 'Анна'

  // 1. The GM is signed in (the account is created on the first run).
  const { context: gmContext } = await setupGM(browser)

  // 2. The GM starts a game over REST.
  const campaignRes = await gmContext.request.post('/api/campaigns', { data: { name: 'E2E' } })
  expect(campaignRes.status()).toBe(201)
  const campaign = (await campaignRes.json()) as { id: string }
  const sessionRes = await gmContext.request.post(`/api/campaigns/${campaign.id}/game-sessions`)
  expect(sessionRes.status()).toBe(201)
  const gameSession = (await sessionRes.json()) as { id: string; link: string }
  const oldLink = gameSession.link

  // 3. The player registers by the game link.
  const { context: playerContext, page: player } = await registerPlayer(
    browser,
    oldLink,
    'E2E',
    'anna',
    playerName,
  )
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

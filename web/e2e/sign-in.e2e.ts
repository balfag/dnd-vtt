import ru from '../src/i18n/ru.json' with { type: 'json' }
import { expect, test, registerPlayer, setupGM, startGame } from './helpers'

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

test('safety screen comes before the sign-in form every time', async ({ browser }) => {
  const { context: gmContext, page: gm } = await setupGM(browser)
  const link = await startGame(gm, 'E2E safety')
  const host = new URL(link).host

  const context = await browser.newContext()
  const page = await context.newPage()
  const signInTab = page.getByRole('tab', { name: ru.play.tabSignIn })
  await page.goto(link)
  await expect(page.getByRole('heading', { name: ru.play.safety.title })).toBeVisible()
  await expect(page.getByText(host, { exact: true })).toBeVisible()
  await expect(page.getByText(ru.play.safety.neverDownload)).toBeVisible()
  await expect(page.getByText(ru.play.safety.neverPasswords)).toBeVisible()
  await expect(
    page.getByText('https://github.com/balfag/dnd-vtt#безопасность-для-игроков'),
  ).toBeVisible()
  await expect(signInTab).toHaveCount(0)
  await expect(page.getByRole('button', { name: ru.play.safety.accept })).toBeFocused()

  await page.getByRole('button', { name: ru.play.safety.accept }).click()
  await expect(signInTab).toBeVisible()
  await expect(signInTab).toBeFocused()

  await page.reload()
  await expect(page.getByRole('heading', { name: ru.play.safety.title })).toBeVisible()
  await expect(signInTab).toHaveCount(0)

  await context.close()
  await gmContext.close()
})

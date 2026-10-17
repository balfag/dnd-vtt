import { randomBytes } from 'node:crypto'
import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test'
import ru from '../src/i18n/ru.json' with { type: 'json' }

export interface Signed {
  context: BrowserContext
  page: Page
}

export interface Player extends Signed {
  password: string
}

const gmLogin = 'master'
// The GM account lives as long as the server of the run; the first test to need it creates it.
let gmPassword: string | undefined

export function password(): string {
  return randomBytes(8).toString('hex')
}

/** setupGM returns a signed-in GM on the campaigns screen: it creates the account on the first call, later ones log in. */
export async function setupGM(browser: Browser): Promise<Signed> {
  const context = await browser.newContext()
  const page = await context.newPage()
  if (gmPassword === undefined) {
    const setupURL = process.env.DNDVTT_E2E_SETUP_URL
    if (setupURL === undefined) throw new Error('DNDVTT_E2E_SETUP_URL is not set')
    const created = password()
    await page.goto(setupURL)
    await page.getByLabel(ru.common.login, { exact: true }).fill(gmLogin)
    await page.getByLabel(ru.common.password, { exact: true }).fill(created)
    await page.getByLabel(ru.common.passwordRepeat, { exact: true }).fill(created)
    await page.getByRole('button', { name: ru.setup.submit }).click()
    gmPassword = created
  } else {
    await page.goto('/login')
    await page.getByLabel(ru.common.login, { exact: true }).fill(gmLogin)
    await page.getByLabel(ru.common.password, { exact: true }).fill(gmPassword)
    await page.getByRole('button', { name: ru.login.submit }).click()
  }
  await expect(page.getByRole('heading', { name: ru.campaigns.title })).toBeVisible()
  return { context, page }
}

/** startGame creates a campaign and starts its game through the campaigns screen; it returns the game link. */
export async function startGame(page: Page, campaignName: string): Promise<string> {
  await page.getByRole('button', { name: ru.campaigns.new }).click()
  await page.getByRole('textbox', { name: ru.campaigns.new }).fill(campaignName)
  await page.getByRole('textbox', { name: ru.campaigns.new }).press('Enter')
  await page
    .getByRole('row')
    .filter({ hasText: campaignName })
    .getByRole('button', { name: ru.campaigns.start })
    .click()
  const linkField = page.getByLabel(ru.game.linkLabel, { exact: true })
  // A game that is still running in another campaign asks for a confirmation first.
  const confirm = page.getByRole('dialog').getByRole('button', { name: ru.campaigns.start })
  await expect(linkField.or(confirm)).toBeVisible()
  if (await confirm.isVisible()) await confirm.click()
  await expect(linkField).toBeVisible()
  return linkField.inputValue()
}

/** registerPlayer opens the game link in a fresh browser and registers a new player; it waits for the table. */
export async function registerPlayer(
  browser: Browser,
  link: string,
  campaignName: string,
  login: string,
  displayName: string,
): Promise<Player> {
  const playerPassword = password()
  const context = await browser.newContext()
  const page = await context.newPage()
  await page.goto(link)
  await expect(page.getByText(campaignName, { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: ru.play.tabRegister }).click()
  await page.getByLabel(ru.common.login, { exact: true }).fill(login)
  await page.getByLabel(ru.common.displayName, { exact: true }).fill(displayName)
  await page.getByLabel(ru.common.password, { exact: true }).fill(playerPassword)
  await page.getByLabel(ru.common.passwordRepeat, { exact: true }).fill(playerPassword)
  await page.getByRole('button', { name: ru.play.submitRegister }).click()
  await expect(page).toHaveURL(/\/table$/)
  return { context, page, password: playerPassword }
}

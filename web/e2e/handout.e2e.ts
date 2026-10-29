import { expect, test } from '@playwright/test'
import ru from '../src/i18n/ru.json' with { type: 'json' }
import { registerPlayer, setupGM, startGame } from './helpers'

const campaignName = 'Записка-кампания'
const pageTitle = 'Записка из комнаты'
const pageText = 'Ключ у хозяйки. Внизу ждут не крысы.'

test('gm shows a selected fragment of a wiki page to one player', async ({ browser }) => {
  // 1. The GM starts a game, two players join.
  const { context: gmContext, page: gm } = await setupGM(browser)
  const link = await startGame(gm, campaignName)
  const { context: annaContext, page: anna } = await registerPlayer(
    browser,
    link,
    campaignName,
    'handout-anna',
    'Анна',
  )
  const { context: borisContext, page: boris } = await registerPlayer(
    browser,
    link,
    campaignName,
    'handout-boris',
    'Борис',
  )

  // 2. On the table the GM opens the wiki panel and writes a page.
  await gm.getByRole('button', { name: ru.table.open }).click()
  await expect(gm).toHaveURL(/\/table$/)
  await gm.getByRole('tab', { name: ru.wiki.tree.tab, exact: true }).click()
  await gm.getByRole('button', { name: ru.wiki.tree.add }).click()
  await gm.getByRole('menuitem', { name: ru.wiki.tools.templates.blank }).click()
  await gm.getByRole('textbox', { name: ru.wiki.page.title }).fill(pageTitle)
  const editor = gm.locator('[contenteditable="true"]').last()
  await editor.click()
  await gm.keyboard.type(pageText)

  // 3. The GM selects the text and shows it to Anna alone.
  await gm.keyboard.press('Shift+Home')
  await gm.getByRole('button', { name: ru.handout.show.button }).click()
  const dialog = gm.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: ru.handout.show.title })).toBeVisible()
  await expect(dialog.getByLabel(ru.handout.show.titleField)).toHaveValue(pageTitle)
  await expect(dialog.getByText(pageText)).toBeVisible()
  await dialog.getByRole('radio', { name: ru.handout.show.selected }).check()
  await dialog.getByRole('checkbox', { name: 'Анна' }).check()
  await dialog
    .getByRole('button', { name: ru.handout.show.sendOne.replace('{{name}}', 'Анна') })
    .click()
  await expect(dialog).toBeHidden()

  // 4. Anna gets the window with the text and closes it.
  const window = anna.getByRole('dialog', { name: pageTitle })
  await expect(window).toBeVisible()
  await expect(window.getByText(pageText)).toBeVisible()
  await expect(window.getByText(ru.handout.modal.fromPrivate)).toBeVisible()
  await expect(window.getByRole('button', { name: ru.handout.modal.close })).toBeFocused()
  await window.getByRole('button', { name: ru.handout.modal.close }).click()
  await expect(window).toBeHidden()

  // 5. The handout stays in her chat; "Open" shows the window again.
  const open = anna.getByRole('button', {
    name: ru.handout.row.openLabel.replace('{{title}}', pageTitle),
  })
  await expect(open).toBeVisible()
  await open.click()
  await expect(window).toBeVisible()
  await expect(window.getByText(pageText)).toBeVisible()
  await window.getByRole('button', { name: ru.handout.modal.close }).click()
  await expect(window).toBeHidden()

  // 6. Boris sees neither the window nor a row of the private handout.
  await expect(boris.getByRole('dialog')).toHaveCount(0)
  await expect(boris.getByRole('button', { name: ru.handout.row.open })).toHaveCount(0)

  await gmContext.close()
  await annaContext.close()
  await borisContext.close()
})

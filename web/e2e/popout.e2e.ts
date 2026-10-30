import ru from '../src/i18n/ru.json' with { type: 'json' }
import { expect, test, setupGM, startGame } from './helpers'

const campaignName = 'Окно-кампания'

test('panel menus work in a popout window', async ({ browser }) => {
  // 1. The GM starts a game and opens the table.
  const { context, page: gm } = await setupGM(browser)
  await startGame(gm, campaignName)
  await gm.getByRole('button', { name: ru.table.open }).click()
  await expect(gm).toHaveURL(/\/table$/)

  // 2. The GM closes the combat panel, then moves the wiki into a window of its own.
  const tab = (page: typeof gm, title: string) => page.locator('.wtab').filter({ hasText: title })
  await tab(gm, ru.combat.tab).click()
  await tab(gm, ru.combat.tab).getByRole('button', { name: ru.workspace.menu }).click()
  await gm.getByRole('menuitem', { name: ru.workspace.close }).click()
  await expect(tab(gm, ru.combat.tab)).toHaveCount(0)

  await tab(gm, ru.wiki.tree.tab).click()
  await tab(gm, ru.wiki.tree.tab).getByRole('button', { name: ru.workspace.menu }).click()
  const popupPromise = context.waitForEvent('page')
  await gm.getByRole('menuitem', { name: ru.workspace.popout }).click()
  const popup = await popupPromise
  await expect(tab(popup, ru.wiki.tree.tab)).toBeVisible()

  // 3. In the window "+" opens the combat panel there.
  await popup.getByRole('button', { name: ru.workspace.add }).click()
  await popup.getByRole('menuitem', { name: ru.combat.tab }).click()
  await expect(tab(popup, ru.combat.tab)).toBeVisible()

  // 4. The tab menu of the combat panel works in the window too: a choice runs its action.
  const menuButton = tab(popup, ru.combat.tab).getByRole('button', { name: ru.workspace.menu })
  await menuButton.click()
  await popup.getByRole('menuitem', { name: ru.workspace.maximize }).click()
  await expect(popup.getByRole('menu')).toHaveCount(0)
  await menuButton.click()
  await popup.getByRole('menuitem', { name: ru.workspace.close }).click()
  await expect(tab(popup, ru.combat.tab)).toHaveCount(0)

  // 5. In the wiki tree of the window the row menu opens and creates a child page.
  await tab(popup, ru.wiki.tree.tab).click()
  await popup.getByRole('button', { name: ru.wiki.tree.add }).click()
  await popup.getByRole('menuitem', { name: ru.wiki.tools.templates.blank }).click()
  const rows = popup.getByRole('treeitem')
  await expect(rows).toHaveCount(1)
  await rows
    .first()
    .getByRole('button', { name: /^Действия со страницей/ })
    .click()
  await popup.getByRole('menuitem', { name: ru.wiki.tree.addChild }).click()
  // The child page asks for a template first.
  await popup.getByRole('menuitem', { name: ru.wiki.tools.templates.blank }).click()
  await expect(popup.getByRole('treeitem')).toHaveCount(2)

  await context.close()
})

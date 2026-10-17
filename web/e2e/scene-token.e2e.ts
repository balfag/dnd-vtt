import { expect, test, type Page } from '@playwright/test'
import ru from '../src/i18n/ru.json' with { type: 'json' }
import { registerPlayer, setupGM, startGame } from './helpers'

const campaignName = 'Склеп-кампания'
const sceneName = 'Склеп'

/** tokenList is the invisible list of the tokens on the scene. */
function tokenList(page: Page) {
  return page.getByRole('list', { name: ru.table.tokens.label })
}

/** itemText is the text of a list item of an unnamed token in the column and the row (from 1). */
function itemText(col: number, row: number): string {
  return ru.table.tokens.item
    .replace('{{name}}', ru.table.tokens.unnamed)
    .replace('{{cell}}', `${String(col)}:${String(row)}`)
}

test('gm places a token, the player sees it move and vanish when it is hidden', async ({
  browser,
}) => {
  // 1. The GM starts a game through the campaigns screen.
  const { context: gmContext, page: gm } = await setupGM(browser)
  const link = await startGame(gm, campaignName)

  // 2. The GM opens the table and creates the first scene; it is shown to the players at once.
  await gm.getByRole('button', { name: ru.table.open }).click()
  await expect(gm).toHaveURL(/\/table$/)
  await gm.getByRole('tab', { name: ru.scenes.tab }).click()
  await gm.getByRole('button', { name: ru.scenes.new }).click()
  const sceneField = gm.getByRole('textbox', { name: ru.scenes.new })
  await sceneField.fill(sceneName)
  await sceneField.press('Enter')
  const canvas = gm.getByRole('application', {
    name: ru.table.canvas.label.replace('{{name}}', sceneName),
  })
  await expect(canvas).toBeVisible()

  // 3. The GM puts a token without an image in the middle of the canvas.
  await gm.getByRole('button', { name: ru.tokens.tool.place }).click()
  const box = await canvas.boundingBox()
  if (box === null) throw new Error('the canvas has no box')
  await gm.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  const gmItems = tokenList(gm).getByRole('listitem')
  await expect(gmItems).toHaveCount(1)
  const placed = (await gmItems.first().textContent()) ?? ''
  const cell = /(\d+):(\d+)$/.exec(placed)
  if (cell?.[1] === undefined || cell[2] === undefined) {
    throw new Error(`no cell in the token list item: ${placed}`)
  }
  const col = Number(cell[1])
  const row = Number(cell[2])
  await expect(gmItems.first()).toHaveText(itemText(col, row))

  // 4. The player enters by the link and sees the token in the same cell.
  const { context: playerContext, page: player } = await registerPlayer(
    browser,
    link,
    campaignName,
    'boris',
    'Борис',
  )
  const playerItems = tokenList(player).getByRole('listitem')
  await expect(playerItems).toHaveCount(1)
  await expect(playerItems.first()).toHaveText(itemText(col, row))

  // 5. The GM lets go of the place tool, selects the token by the keyboard and moves it a cell right.
  await gm.keyboard.press('Escape')
  await canvas.focus()
  await gm.keyboard.press('n')
  await gm.keyboard.press('ArrowRight')
  await expect(gmItems.first()).toHaveText(itemText(col + 1, row))

  // 6. The player sees the move.
  await expect(playerItems.first()).toHaveText(itemText(col + 1, row))

  // 7. The GM hides the token in its properties: the player's list gets empty.
  await gm.keyboard.press('Enter')
  const dialog = gm.getByRole('dialog')
  await dialog.getByLabel(ru.tokens.field.hidden, { exact: true }).check()
  await dialog.getByRole('button', { name: ru.tokens.save }).click()
  await expect(dialog).toBeHidden()
  await expect(playerItems).toHaveCount(0)
  await expect(gmItems).toHaveCount(1)

  await gmContext.close()
  await playerContext.close()
})

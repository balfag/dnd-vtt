import { MessageType, type Token } from '../../shared/protocol'
import { zOrder } from './tokenForm'

/** TokenAction is a quick action of the GM on a token; the key and the menu item do the same. */
export type TokenAction =
  'properties' | 'toggleHidden' | 'toggleLocked' | 'toFront' | 'toBack' | 'delete'

type Command = (type: string, data?: unknown) => Promise<unknown>

/** TokenMenuItem is a row of the context menu: the i18n key of its text and the key that does the same. */
export interface TokenMenuItem {
  action: TokenAction
  label: string
  shortcut: string
}

/** tokenMenuItems lists the actions for the token; hide and lock name what the action does now. */
export function tokenMenuItems(token: Token): TokenMenuItem[] {
  return [
    { action: 'properties', label: 'tokens.menu.properties', shortcut: 'Enter' },
    {
      action: 'toggleHidden',
      label: token.hidden ? 'tokens.menu.show' : 'tokens.menu.hide',
      shortcut: 'H',
    },
    {
      action: 'toggleLocked',
      label: token.locked ? 'tokens.menu.unlock' : 'tokens.menu.lock',
      shortcut: 'L',
    },
    { action: 'toFront', label: 'tokens.menu.toFront', shortcut: ']' },
    { action: 'toBack', label: 'tokens.menu.toBack', shortcut: '[' },
    { action: 'delete', label: 'tokens.menu.delete', shortcut: 'Delete' },
  ]
}

/** actionOfKey is the action a key does on the selected token, or null. */
export function actionOfKey(key: string): TokenAction | null {
  switch (key) {
    case 'Enter':
      return 'properties'
    case 'h':
    case 'H':
      return 'toggleHidden'
    case 'l':
    case 'L':
      return 'toggleLocked'
    case ']':
      return 'toFront'
    case '[':
      return 'toBack'
    case 'Delete':
      return 'delete'
    default:
      return null
  }
}

/**
 * runTokenAction sends the command of an action that needs no dialog and resolves with the
 * i18n key of what to announce. A refusal of the server rejects.
 */
export async function runTokenAction(
  action: 'toggleHidden' | 'toggleLocked' | 'toFront' | 'toBack',
  token: Token,
  tokens: readonly Token[],
  command: Command,
): Promise<string> {
  switch (action) {
    case 'toggleHidden':
      await command(MessageType.TokenUpdate, { tokenId: token.id, hidden: !token.hidden })
      return token.hidden ? 'tokens.announce.shown' : 'tokens.announce.hidden'
    case 'toggleLocked':
      await command(MessageType.TokenUpdate, { tokenId: token.id, locked: !token.locked })
      return token.locked ? 'tokens.announce.unlocked' : 'tokens.announce.locked'
    case 'toFront':
      await command(MessageType.TokenUpdate, {
        tokenId: token.id,
        z: zOrder(token, tokens, 'front'),
      })
      return 'tokens.announce.front'
    case 'toBack':
      await command(MessageType.TokenUpdate, {
        tokenId: token.id,
        z: zOrder(token, tokens, 'back'),
      })
      return 'tokens.announce.back'
  }
}

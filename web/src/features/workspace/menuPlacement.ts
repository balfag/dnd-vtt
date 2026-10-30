/** MENU_MARGIN is the gap, in pixels, kept between a popup menu and the edge of the window. */
export const MENU_MARGIN = 8

interface Size {
  width: number
  height: number
}

/**
 * placeMenu returns the top-left corner of a popup menu opened at the point `at`.
 * The alignment is the intent: "end" puts the right-hand corner of the menu at `at.x`.
 * The menu is moved only when it would not fit in the viewport with the margin around it.
 */
export function placeMenu(
  at: { x: number; y: number },
  align: 'start' | 'end',
  menu: Size,
  viewport: Size,
): { left: number; top: number } {
  let left = align === 'end' ? at.x - menu.width : at.x
  if (left + menu.width > viewport.width - MENU_MARGIN) {
    left = viewport.width - MENU_MARGIN - menu.width
  }
  if (left < MENU_MARGIN) left = MENU_MARGIN

  let top = at.y
  if (top + menu.height > viewport.height - MENU_MARGIN) {
    top = at.y - menu.height - MENU_MARGIN
  }
  if (top < MENU_MARGIN) top = MENU_MARGIN

  return { left, top }
}

/** maxIconPoints is the longest icon of a page, in code points (the server's ValidIcon). */
export const maxIconPoints = 8

/** validIcon tells whether a page icon is acceptable: empty, or up to 8 code points without control characters. */
export function validIcon(icon: string): boolean {
  if (Array.from(icon).length > maxIconPoints) return false
  // eslint-disable-next-line no-control-regex
  return !/[\u0000-\u001f\u007f-\u009f]/.test(icon)
}

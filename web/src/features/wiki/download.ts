/**
 * download makes the browser save the file at url. The request goes out with the cookie of the
 * page, and the server names the file (Content-Disposition), so nothing is read into memory here.
 */
export function download(url: string): void {
  const link = document.createElement('a')
  link.href = url
  link.download = ''
  link.rel = 'noopener'
  document.body.append(link)
  link.click()
  link.remove()
}

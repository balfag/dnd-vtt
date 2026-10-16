/** SocketLike is the part of the browser WebSocket the connection uses. */
export interface SocketLike {
  send(data: string): void
  close(code?: number, reason?: string): void
  onopen: (() => void) | null
  onmessage: ((ev: { data: unknown }) => void) | null
  onclose: ((ev: { code: number }) => void) | null
  onerror: (() => void) | null
}
export type SocketFactory = (url: string) => SocketLike

/** gameSocketUrl returns ws(s)://<host>/ws for the page location. */
export function gameSocketUrl(loc: { protocol: string; host: string }): string {
  const scheme = loc.protocol === 'https:' ? 'wss' : 'ws'
  return `${scheme}://${loc.host}/ws`
}

export const browserSocketFactory: SocketFactory = (url) => {
  const ws = new WebSocket(url)
  const sock: SocketLike = {
    send: (data) => {
      ws.send(data)
    },
    close: (code, reason) => {
      ws.close(code, reason)
    },
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
  }
  ws.onopen = () => sock.onopen?.()
  ws.onmessage = (ev) => sock.onmessage?.({ data: ev.data })
  ws.onclose = (ev) => sock.onclose?.({ code: ev.code })
  ws.onerror = () => sock.onerror?.()
  return sock
}

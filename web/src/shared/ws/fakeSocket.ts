import type { SocketFactory, SocketLike } from './socket'

/** FakeSocket is a scripted socket for tests; it is used only from *.test.ts files. */
export class FakeSocket implements SocketLike {
  readonly url: string
  sent: unknown[] = []
  closedWith: number | undefined = undefined
  onopen: (() => void) | null = null
  onmessage: ((ev: { data: unknown }) => void) | null = null
  onclose: ((ev: { code: number }) => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
  }

  send(data: string): void {
    this.sent.push(JSON.parse(data))
  }

  close(code?: number): void {
    this.closedWith = code
  }

  open(): void {
    this.onopen?.()
  }

  receive(msg: unknown): void {
    this.receiveRaw(JSON.stringify(msg))
  }

  receiveRaw(data: string): void {
    this.onmessage?.({ data })
  }

  serverClose(code: number): void {
    this.onclose?.({ code })
  }
}

export function fakeSocketFactory(): { factory: SocketFactory; sockets: FakeSocket[] } {
  const sockets: FakeSocket[] = []
  const factory: SocketFactory = (url) => {
    const s = new FakeSocket(url)
    sockets.push(s)
    return s
  }
  return { factory, sockets }
}

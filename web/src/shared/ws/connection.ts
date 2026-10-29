import type { StoreApi } from 'zustand/vanilla'
import type { GameState } from '../../features/game/store'
import {
  CloseCode,
  helloCommand,
  isEnvelope,
  isErrorMessage,
  isHelloAck,
  isSnapshot,
  isPingShown,
  isRulerShown,
  isTokenDragging,
  MessageType,
  pingCommand,
  type Envelope,
  type Hello,
} from '../protocol'
import { reconnectDelay } from './backoff'
import { CommandError } from './errors'
import { browserSocketFactory, type SocketFactory, type SocketLike } from './socket'

const pingIntervalMs = 20000
const normalClose = 1000

export interface GameConnectionOptions {
  store: StoreApi<GameState>
  url: string
  createSocket?: SocketFactory
  random?: () => number
  commandTimeoutMs?: number
}

interface Pending {
  resolve: (d: unknown) => void
  reject: (e: CommandError) => void
  timer: ReturnType<typeof setTimeout>
}

/** GameConnection keeps the game WebSocket alive and feeds the game store. */
export class GameConnection {
  private readonly store: StoreApi<GameState>
  private readonly url: string
  private readonly createSocket: SocketFactory
  private readonly random: () => number
  private readonly commandTimeoutMs: number

  private socket: SocketLike | null = null
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private pingTimer: ReturnType<typeof setInterval> | null = null
  private pending = new Map<string, Pending>()
  private helloId: string | null = null
  private attempt = 0
  private helloCount = 0
  private commandCount = 0
  private pingCount = 0

  constructor(opts: GameConnectionOptions) {
    this.store = opts.store
    this.url = opts.url
    this.createSocket = opts.createSocket ?? browserSocketFactory
    this.random = opts.random ?? Math.random
    this.commandTimeoutMs = opts.commandTimeoutMs ?? 10000
  }

  /** start opens the connection; calling it twice is a no-op. */
  start(): void {
    if (this.socket !== null || this.reconnectTimer !== null) return
    this.attempt = 0
    this.connect()
  }

  /**
   * stop closes the socket with 1000, cancels timers, rejects pending commands with
   * 'disconnected', and sets connection to idle. The instance can be started again.
   */
  stop(): void {
    this.dropSocket(true)
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.store.getState().setConnection({ state: 'idle' })
  }

  /** command sends {t, id, d} and resolves with ack.d (undefined when absent). */
  command(t: string, d?: unknown): Promise<unknown> {
    const socket = this.socket
    if (socket === null || !this.isSynced()) {
      return Promise.reject(new CommandError('disconnected'))
    }
    this.commandCount++
    const id = `c-${String(this.commandCount)}`
    return new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new CommandError('timeout'))
      }, this.commandTimeoutMs)
      this.pending.set(id, { resolve, reject, timer })
      socket.send(JSON.stringify({ t, id, d }))
    })
  }

  /** transient sends {t, d} without id when synced; otherwise drops it silently. */
  transient(t: string, d?: unknown): void {
    if (this.socket === null || !this.isSynced()) return
    this.socket.send(JSON.stringify({ t, d }))
  }

  private isSynced(): boolean {
    return this.store.getState().connection.state === 'synced'
  }

  private connect(): void {
    this.store.getState().setConnection({ state: 'connecting', attempt: this.attempt })
    const sock = this.createSocket(this.url)
    this.socket = sock
    sock.onopen = () => {
      if (this.socket !== sock) return
      this.sendHello(sock)
    }
    sock.onmessage = (ev) => {
      if (this.socket !== sock) return
      this.handleMessage(sock, ev.data)
    }
    sock.onclose = (ev) => {
      if (this.socket !== sock) return
      this.handleClose(ev.code)
    }
    sock.onerror = () => {
      // A close event always follows; the reconnect logic lives there.
    }
  }

  private sendHello(sock: SocketLike): void {
    this.helloCount++
    this.helloId = `h-${String(this.helloCount)}`
    const state = this.store.getState()
    const hello: Hello =
      state.game === null ? {} : { gameSessionId: state.game.gameSessionId, lastSeq: state.lastSeq }
    sock.send(JSON.stringify(helloCommand(this.helloId, hello)))
  }

  private handleMessage(sock: SocketLike, raw: unknown): void {
    let msg: unknown
    try {
      msg = typeof raw === 'string' ? JSON.parse(raw) : undefined
    } catch {
      msg = undefined
    }
    if (!isEnvelope(msg)) {
      console.warn('ws: dropped a malformed message')
      return
    }

    const env: Envelope = msg
    const seq = env.seq
    if (isHelloAck(env) && env.re === this.helloId) {
      this.helloId = null
      if (env.d.resumed) this.markSynced(sock)
      return
    }
    if (isSnapshot(env)) {
      this.store.getState().applySnapshot(env.d)
      if (!this.isSynced()) this.markSynced(sock)
      return
    }
    if ((env.t === MessageType.Ack || env.t === MessageType.Error) && env.re !== undefined) {
      this.settle(env, env.re)
      return
    }
    if (seq !== undefined) {
      this.handleEvent(env, seq)
      return
    }
    // Transient messages have no seq: they bypass the order checks and are never replayed.
    const transient = isTokenDragging(env) || isRulerShown(env) || isPingShown(env)
    if (transient && this.isSynced()) this.store.getState().applyTransient(env)
  }

  private settle(msg: Envelope, re: string): void {
    const p = this.pending.get(re)
    if (p === undefined) return
    this.pending.delete(re)
    clearTimeout(p.timer)
    if (msg.t === MessageType.Ack) {
      p.resolve(msg.d)
    } else if (isErrorMessage(msg)) {
      p.reject(new CommandError(msg.d.code, msg.d.message))
    } else {
      console.warn('ws: dropped a malformed error message')
      p.reject(new CommandError('internal'))
    }
  }

  private handleEvent(msg: Envelope, seq: number): void {
    if (!this.isSynced()) return
    const last = this.store.getState().lastSeq
    if (seq <= last) return
    if (seq === last + 1) {
      this.store.getState().applyEvent({ ...msg, seq })
      return
    }
    // A gap: resync from scratch right away.
    this.dropSocket(true)
    this.attempt++
    this.connect()
  }

  private markSynced(sock: SocketLike): void {
    this.attempt = 0
    this.store.getState().setConnection({ state: 'synced' })
    this.stopPing()
    this.pingTimer = setInterval(() => {
      this.pingCount++
      sock.send(JSON.stringify(pingCommand(`p-${String(this.pingCount)}`)))
    }, pingIntervalMs)
  }

  private handleClose(code: number): void {
    this.dropSocket(false)
    if (code === CloseCode.SessionRevoked) {
      this.store.getState().setConnection({ state: 'closed', reason: 'unauthorized' })
    } else if (code === CloseCode.PlayerRemoved) {
      this.store.getState().setConnection({ state: 'closed', reason: 'removed' })
    } else if (code === CloseCode.GameEnded) {
      this.store.getState().setConnection({ state: 'closed', reason: 'gameEnded' })
    } else {
      if (code === 1003 || code === 1009) {
        console.error(`ws: closed by the server with ${String(code)}`)
      }
      this.attempt++
      const attempt = this.attempt
      const delayMs = reconnectDelay(attempt, this.random)
      this.store.getState().setConnection({ state: 'reconnecting', attempt, delayMs })
      this.reconnectTimer = setTimeout(() => {
        this.reconnectTimer = null
        this.connect()
      }, delayMs)
    }
  }

  /** dropSocket detaches the current socket, stops pings and fails pending commands. */
  private dropSocket(closeIt: boolean): void {
    const sock = this.socket
    this.socket = null
    this.helloId = null
    this.stopPing()
    for (const p of this.pending.values()) {
      clearTimeout(p.timer)
      p.reject(new CommandError('disconnected'))
    }
    this.pending.clear()
    if (sock !== null && closeIt) sock.close(normalClose)
  }

  private stopPing(): void {
    if (this.pingTimer !== null) {
      clearInterval(this.pingTimer)
      this.pingTimer = null
    }
  }
}

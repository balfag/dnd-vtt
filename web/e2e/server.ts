import { spawn, type ChildProcess } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { createInterface } from 'node:readline'

export interface E2EServer {
  baseURL: string // http://localhost:<port>
  setupURL: string // the first-run link printed by the binary
  stop(): Promise<void>
}

const startTimeoutMs = 20_000
const stopTimeoutMs = 10_000
const setupLine = /^First run: create the GM account at (\S+)$/

/** binaryPath returns the server binary under test or throws with a build hint. */
export function binaryPath(): string {
  const fromEnv = process.env.DNDVTT_E2E_BIN
  const bin =
    fromEnv !== undefined && fromEnv !== ''
      ? fromEnv
      : resolve(
          import.meta.dirname,
          '..',
          '..',
          'out',
          process.platform === 'win32' ? 'dndvtt.exe' : 'dndvtt',
        )
  if (!existsSync(bin)) {
    throw new Error(
      `e2e server binary not found: ${bin}. Build it from the repo root: ` +
        '`pnpm -C web build` then `go build -o out/dndvtt ./cmd/dndvtt` ' +
        '(or set DNDVTT_E2E_BIN).',
    )
  }
  return bin
}

/** startServer runs the binary with a fresh data dir and waits for the first-run link. */
export async function startServer(binary: string): Promise<E2EServer> {
  const dataDir = await mkdtemp(join(tmpdir(), 'dndvtt-e2e-'))
  const child = spawn(
    binary,
    ['-dev', '-http-addr', '127.0.0.1:0', '-data-dir', dataDir, '-log-level', 'warn'],
    { stdio: ['ignore', 'pipe', 'pipe'] },
  )

  const exited = new Promise<void>((done) => {
    child.once('exit', () => {
      done()
    })
  })
  const stop = async (): Promise<void> => {
    if (isRunning(child)) {
      // Windows has no POSIX signals; kill() terminates the process there.
      child.kill(process.platform === 'win32' ? undefined : 'SIGTERM')
      await Promise.race([exited, delay(stopTimeoutMs)])
      if (isRunning(child)) child.kill('SIGKILL')
    }
    await rm(dataDir, { recursive: true, force: true })
  }

  const tail: string[] = []
  const remember = (line: string) => {
    tail.push(line)
    if (tail.length > 20) tail.shift()
  }

  try {
    const setupURL = await waitForSetupURL(child, remember)
    return { baseURL: new URL(setupURL).origin, setupURL, stop }
  } catch (err) {
    await stop()
    const reason = err instanceof Error ? err.message : String(err)
    throw new Error(`${reason}\nlast output:\n${tail.join('\n')}`, { cause: err })
  }
}

function waitForSetupURL(child: ChildProcess, remember: (line: string) => void): Promise<string> {
  return new Promise<string>((resolveURL, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`no first-run link within ${String(startTimeoutMs / 1000)}s`))
    }, startTimeoutMs)

    const settle = (fn: () => void) => {
      clearTimeout(timer)
      fn()
    }

    if (child.stdout !== null) {
      createInterface({ input: child.stdout }).on('line', (line) => {
        remember(line)
        const m = setupLine.exec(line)
        if (m?.[1] !== undefined) {
          const url = m[1]
          settle(() => {
            resolveURL(url)
          })
        }
      })
    }
    if (child.stderr !== null) {
      createInterface({ input: child.stderr }).on('line', remember)
    }
    child.once('error', (err) => {
      settle(() => {
        reject(err)
      })
    })
    child.once('exit', (code, signal) => {
      settle(() => {
        reject(new Error(`server exited early (code ${String(code)}, signal ${String(signal)})`))
      })
    })
  })
}

function isRunning(child: ChildProcess): boolean {
  return child.exitCode === null && child.signalCode === null
}

function delay(ms: number): Promise<void> {
  return new Promise((done) => setTimeout(done, ms))
}

let running: E2EServer | undefined

/** setRunningServer remembers the server for globalTeardown. */
export function setRunningServer(server: E2EServer): void {
  running = server
}

/** stopRunningServer stops the remembered server, if any. */
export async function stopRunningServer(): Promise<void> {
  const server = running
  running = undefined
  await server?.stop()
}

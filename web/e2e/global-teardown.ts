import { stopRunningServer } from './server'

/** Stops the server started by global-setup.ts and removes its data dir. */
export default async function globalTeardown(): Promise<void> {
  await stopRunningServer()
}

import { binaryPath, setRunningServer, startServer } from './server'

/** Starts the real binary once for the whole run and exposes its URLs to the workers. */
export default async function globalSetup(): Promise<void> {
  const server = await startServer(binaryPath())
  setRunningServer(server)
  process.env.DNDVTT_E2E_BASE_URL = server.baseURL
  process.env.DNDVTT_E2E_SETUP_URL = server.setupURL
}

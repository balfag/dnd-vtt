import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // The suffix keeps Vitest (*.test.ts, *.spec.ts) away from these files.
  testMatch: '**/*.e2e.ts',
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 30_000,
  use: {
    baseURL: process.env.DNDVTT_E2E_BASE_URL,
    locale: 'ru-RU',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
})

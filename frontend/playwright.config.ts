import { defineConfig, devices } from '@playwright/test'

const port = process.env.E2E_APP_PORT ?? '18089'
const baseURL = `http://localhost:${port}`
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 8_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: { baseURL, actionTimeout: 10_000, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: process.env.E2E_ENGINE ?? 'mysql', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: './dblama',
    cwd: '..',
    env: { LISTEN: `127.0.0.1:${port}`, LOGIN_RATE_LIMIT: '1000/min', TRUSTED_SERVERS: `${process.env.E2E_DB_HOST ?? '127.0.0.1'}:${process.env.E2E_DB_PORT}` },
    url: `${baseURL}/api/drivers`,
    reuseExistingServer: false,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 5000 },
  },
})

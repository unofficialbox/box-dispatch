import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  // The mock backend is one stateful in-memory workspace shared by the suite.
  // Serialize browser tests so one deployment cannot replace another test's plan.
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:8788',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'go run ./cmd/box-dispatch mock --no-open --port 8788',
    cwd: '..',
    url: 'http://127.0.0.1:8788/api/health',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
})

import { defineConfig, devices } from '@playwright/test';
import { reporters, sharedUse, vaneCommand } from './playwright.shared';

/**
 * Gate 7 (Production Acceptance Testing, see
 * vane-private/internal_docs/1.0.0/production-acceptance-suite.md): drives
 * examples/fullstack-app end-to-end against its real backend, as a separate
 * suite from tests/e2e (see playwright.shared.ts for why it's a separate
 * config, not a second project/webServer entry in the main one).
 */
export default defineConfig({
  testDir: './tests/e2e-acceptance/specs',
  timeout: 60 * 1000,
  expect: {
    timeout: 10000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  /* Always 1, unlike playwright.config.ts: every spec shares the same
   * in-memory API store (examples/fullstack-app/api/store.go), so parallel
   * workers would step on each other's users/notes. */
  workers: 1,
  reporter: reporters('test-results/acceptance/playwright.json', 'playwright-report/acceptance'),
  use: {
    ...sharedUse,
    baseURL: 'http://localhost:4174',
  },

  /* Chromium only for now: this suite is slower (real backend, real WASM
   * boot) than tests/e2e's per-spec cost, and the full 6-project browser
   * matrix multiplying that is still an open question (see the doc's
   * "Open questions" section) rather than a settled decision. */
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  webServer: [
    {
      command: 'go run ./examples/fullstack-app/api',
      port: 8081,
      reuseExistingServer: !process.env.CI,
      timeout: 60 * 1000,
    },
    {
      command: `${vaneCommand} run examples/fullstack-app --port 4174`,
      url: 'http://localhost:4174',
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
    },
  ],
});

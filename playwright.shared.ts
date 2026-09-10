import type { PlaywrightTestConfig } from '@playwright/test';

/**
 * Shared between playwright.config.ts (tests/e2e, the synthetic feature-probe
 * app) and playwright.acceptance.config.ts (tests/e2e-acceptance, the real
 * fullstack-app product suite). Kept as two separate config files rather than
 * one merged config: Playwright's webServer list isn't scoped per project, so
 * a single config would boot the fullstack-app's API + frontend on every run
 * of the unrelated main suite too.
 */
export function reporters(jsonFile: string, htmlOutputFolder?: string): PlaywrightTestConfig['reporter'] {
  const html: ['html', { outputFolder?: string }] = ['html', htmlOutputFolder ? { outputFolder: htmlOutputFolder } : {}];
  return process.env.CI
    ? [['github'], html, ['json', { outputFile: jsonFile }]]
    : [html, ['json', { outputFile: jsonFile }]];
}

export const sharedUse: PlaywrightTestConfig['use'] = {
  trace: 'on-first-retry',
  screenshot: 'only-on-failure',
  video: 'retain-on-failure',
};

export const vaneCommand = process.platform === 'win32' ? 'vane.exe' : './vane';

import { expect, test } from '@playwright/test'

test('boots the WASM application', async ({ page }) => {
  await page.goto('/')

  await expect(
    page.getByTestId('app-ready')
  ).toHaveText('ready')
})

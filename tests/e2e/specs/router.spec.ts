import { expect, test } from '@playwright/test'

test('navigates between fixture pages', async ({ page }) => {
  await page.goto('/#/smoke')

  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page).toHaveTitle('Smoke - Vane E2E')

  await page.getByRole('link', { name: 'Signals' }).click()

  await expect(page).toHaveURL(/#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
  await expect(page).toHaveTitle('Signals - Vane E2E')
})

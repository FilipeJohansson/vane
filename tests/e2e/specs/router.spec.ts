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

test('renders a route parameter and the built-in 404 fallback', async ({ page }) => {
  await page.goto('/#/users/42')

  await expect(page.getByTestId('user-id')).toHaveText('42')
  await expect(page).toHaveTitle('User - Vane E2E')

  await page.goto('/#/missing')

  await expect(page.getByText('404')).toBeVisible()
})

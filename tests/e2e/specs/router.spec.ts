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

test('boots the fixture from the root route', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page).toHaveTitle('Smoke - Vane E2E')
})

test('renders a route parameter and the built-in 404 fallback', async ({ page }) => {
  await page.goto('/#/users/42')

  await expect(page.getByTestId('user-id')).toHaveText('42')
  await expect(page).toHaveTitle('User - Vane E2E')

  await page.goto('/#/missing')

  await expect(page.getByText('404')).toBeVisible()
})

test('supports direct navigation and refresh', async ({ page }) => {
  await page.goto('/#/signals')
  await expect(page.getByTestId('signal-count')).toHaveText('0')

  await page.reload()
  await expect(page).toHaveURL(/#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('supports browser back and forward', async ({ page }) => {
  await page.goto('/#/smoke')
  await page.getByRole('link', { name: 'Signals' }).click()
  await expect(page).toHaveURL(/#\/signals$/)

  await page.goBack()
  await expect(page).toHaveURL(/#\/smoke$/)
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goForward()
  await expect(page).toHaveURL(/#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('keeps nested layouts mounted while child routes change', async ({ page }) => {
  await page.goto('/#/nested')

  await expect(page.getByTestId('nested-layout')).toBeVisible()
  await expect(page.getByTestId('nested-page')).toHaveText('Nested home')
  await expect(page.getByTestId('layout-mounts')).toHaveText('1')

  await page.getByRole('link', { name: 'Nested detail' }).click()
  await expect(page).toHaveURL(/#\/nested\/detail$/)
  await expect(page.getByTestId('nested-page')).toHaveText('Nested detail')
  await expect(page.getByTestId('layout-mounts')).toHaveText('1')
})

test('router.Replace does not push a history entry, so back skips the replaced route', async ({ page }) => {
  await page.goto('/#/rendering')
  await page.getByRole('link', { name: 'Smoke' }).click()
  await expect(page).toHaveURL(/#\/smoke$/)

  await page.getByTestId('replace-to-signals').click()
  await expect(page).toHaveURL(/#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')

  // If Replace had pushed a new entry instead of replacing, this would land
  // back on /smoke; since it doesn't push, back must skip straight over the
  // replaced entry to whatever preceded it (/rendering).
  await page.goBack()
  await expect(page).toHaveURL(/#\/rendering$/)
})

test('cleans route-scoped content when navigating away', async ({ page }) => {
  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('lifecycle-cleanups')).toHaveText('0')

  await page.goto('/#/smoke')
  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page.getByTestId('portal-host')).toBeEmpty()
})

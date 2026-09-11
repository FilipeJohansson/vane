import { expect, test } from '@playwright/test'

// A pathname under /hash-mode boots the shared fixture with HashLocation
// instead of the router's default PathLocation (see App.vane's hashModeBase
// gate), so these specs run against the same app/routes as router.spec.ts,
// just addressed with a hash fragment instead of real paths. Lighter than
// router.spec.ts on purpose: HashLocation has no click interception (a plain
// <a href="#/x"> is a native browser anchor, the router never needs to
// intercept its click) and no in-page anchor support (HashLocation.AnchorID
// always returns "", since the fragment is the routing mechanism itself) -
// this file only covers what's actually specific to hash-based routing.

test('navigates between fixture pages using a hash fragment', async ({ page }) => {
  await page.goto('/hash-mode#/smoke')

  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page).toHaveURL(/\/hash-mode#\/smoke$/)

  await page.getByRole('link', { name: 'Signals' }).click()

  await expect(page).toHaveURL(/\/hash-mode#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('supports browser back and forward with a hash fragment', async ({ page }) => {
  await page.goto('/hash-mode#/smoke')
  await page.getByRole('link', { name: 'Signals' }).click()
  await expect(page).toHaveURL(/\/hash-mode#\/signals$/)

  await page.goBack()
  await expect(page).toHaveURL(/\/hash-mode#\/smoke$/)
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goForward()
  await expect(page).toHaveURL(/\/hash-mode#\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('supports a direct load and a refresh of a nested route', async ({ page }) => {
  await page.goto('/hash-mode#/users/42')
  await expect(page.getByTestId('user-id')).toHaveText('42')

  await page.reload()

  await expect(page).toHaveURL(/\/hash-mode#\/users\/42$/)
  await expect(page.getByTestId('user-id')).toHaveText('42')
})

// Same scenario as router.spec.ts's mount-guard test, under HashLocation.
test('a mount guard re-fires on browser back after logging out', async ({ page }) => {
  await page.goto('/hash-mode#/guard')
  await expect(page.getByTestId('guard-status')).toBeVisible()

  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/\/hash-mode#\/smoke$/)

  await page.goBack()
  await expect(page).toHaveURL(/\/hash-mode#\/smoke$/)
})

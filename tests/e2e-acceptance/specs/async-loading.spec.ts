import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'
import { sidebarLink } from '../support/nav'

// Both specs intercept only to add latency - route.continue() still sends
// the real request to the real backend and returns its real response, just
// delayed. That's what makes the transient "Loading…" state deterministically
// observable without weakening "against the real API" to "against a mock":
// the fetch and its data are real, only the timing is controlled.
function delay(ms: number) {
  return async (route: import('@playwright/test').Route) => {
    await new Promise((resolve) => setTimeout(resolve, ms))
    await route.continue()
  }
}

test('shows Loading… while the dashboard stats fetch is in flight, then replaces it with real data', async ({
  page,
}) => {
  await registerNewUser(page, 'Async Dashboard')

  await page.route('**/api/stats', delay(1000))
  await page.reload()

  await expect(page.getByText('Loading…').first()).toBeVisible()
  await expect(page.getByText('Total users')).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('Loading…')).toHaveCount(0)
})

test('shows Loading… while the Users list fetch is in flight, then replaces it with the table', async ({ page }) => {
  await registerNewUser(page, 'Async Users List')

  await page.route('**/api/users', delay(1000))
  await sidebarLink(page, 'Users').click()

  await expect(page.getByText('Loading…')).toBeVisible()
  await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('Loading…')).toHaveCount(0)
})

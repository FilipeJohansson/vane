import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'

// Covers UserNotes.vane's core.Try error boundary against real failures - a
// real backend business error here, a real aborted network request in the
// second spec - never a mocked/fabricated error response.

test('a real backend error renders the error boundary instead of crashing the page', async ({ page }) => {
  await registerNewUser(page, 'Error State User')

  // No such user id - the real API's own 404 ("user not found"), not a
  // fabricated response.
  await page.goto('/dashboard/users/999999/notes')

  await expect(page.locator('.notes-error')).toBeVisible()
  await expect(page.locator('.notes-error')).toContainText('user not found')

  // The boundary contained the failure - the rest of the page still works.
  await page.getByRole('link', { name: '← Back to user' }).click()
  await page.waitForURL(/\/dashboard\/users\/999999$/)
})

test('a real network failure is recoverable: Retry succeeds once the network recovers', async ({ page }) => {
  await registerNewUser(page, 'Error State Recover')

  // "View all ->" on the dashboard links to this account's own notes page.
  await page.getByText('View all').click()
  await page.waitForURL(/\/dashboard\/users\/\d+\/notes$/)

  // Genuinely aborted request, not a fabricated response - real network
  // failure, same shape client.go itself wraps as "network error: ...".
  // Scoped to the backend's own origin: a bare '**/notes' also matches this
  // page's own document request (the frontend route itself ends in
  // "/notes" under the router's default PathLocation), which would abort
  // the page load instead of just the API call.
  await page.route('http://localhost:8081/**/notes', async (route) => {
    if (route.request().method() === 'GET') {
      await route.abort()
      return
    }
    await route.continue()
  })

  await page.reload()
  await expect(page.locator('.notes-error')).toBeVisible()
  await expect(page.locator('.notes-error')).toContainText('network error')

  await page.unroute('http://localhost:8081/**/notes')
  await page.getByRole('button', { name: 'Retry' }).click()

  await expect(page.locator('.notes-error')).toHaveCount(0)
  await expect(page.getByText('No notes yet.')).toBeVisible()
})

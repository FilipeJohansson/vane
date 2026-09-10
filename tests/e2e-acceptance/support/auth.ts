import type { Page } from '@playwright/test'

// Shared across acceptance specs: every flow needs a logged-in user, and the
// backend's in-memory store (examples/fullstack-app/api/store.go) persists
// across specs within a run, so each call must produce a unique account.
export async function registerNewUser(page: Page, name: string) {
  const email = `${name.toLowerCase().replace(/\s+/g, '.')}.${Date.now()}.${Math.floor(Math.random() * 1e6)}@test.com`
  const password = 'password123'

  await page.goto('/#/register')
  await page.getByLabel('Name').fill(name)
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByLabel('Confirm password').fill(password)
  await page.getByRole('button', { name: 'Sign up' }).click()
  await page.waitForURL(/#\/dashboard$/)

  return { name, email, password }
}

import { expect, test, type Page } from '@playwright/test'

// examples/fullstack-app uses hash-based routing (router.HashLocation, the
// router package's default) - every URL below needs the leading '#'. See
// production-acceptance-suite.md for where that was first flagged.

async function registerNewUser(page: Page, name: string) {
  const email = `${name.toLowerCase().replace(/\s+/g, '.')}.${Date.now()}.${Math.floor(Math.random() * 1e6)}@test.com`
  const password = 'password123'

  await page.goto('/#/register')
  await page.getByLabel('Name').fill(name)
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByLabel('Confirm password').fill(password)
  await page.getByRole('button', { name: 'Sign up' }).click()
  await page.waitForURL(/#\/dashboard$/)

  return { email, password }
}

test('visiting a protected route while logged out redirects to login', async ({ page }) => {
  await page.goto('/#/dashboard')

  await expect(page).toHaveURL(/#\/login$/)
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible()
})

test('register creates an account and lands on the dashboard', async ({ page }) => {
  await registerNewUser(page, 'Auth Register')

  await expect(page.getByRole('heading', { name: /^Welcome, Auth Register$/ })).toBeVisible()
})

test('logout clears the session and protected routes redirect again', async ({ page }) => {
  await registerNewUser(page, 'Auth Logout')

  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/#\/login$/)

  const token = await page.evaluate(() => localStorage.getItem('fullstack-app.token'))
  expect(token).toBeNull()

  // Session is really gone, not just the redirect from the click itself.
  await page.goto('/#/dashboard')
  await expect(page).toHaveURL(/#\/login$/)
})

test('login with existing credentials reaches the dashboard', async ({ page }) => {
  const { email, password } = await registerNewUser(page, 'Auth Login')
  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/#\/login$/)

  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()

  await page.waitForURL(/#\/dashboard$/)
  await expect(page.getByRole('heading', { name: /^Welcome, Auth Login$/ })).toBeVisible()
})

test('login with wrong password shows an error and stays on the login page', async ({ page }) => {
  const { email } = await registerNewUser(page, 'Auth Bad Login')
  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/#\/login$/)

  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill('wrong-password')
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page.locator('.auth-error')).toBeVisible()
  await expect(page).toHaveURL(/#\/login$/)
})

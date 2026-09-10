import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'

test('register requires matching passwords before it ever submits', async ({ page }) => {
  const email = `forms.mismatch.${Date.now()}@test.com`

  await page.goto('/#/register')
  await page.getByLabel('Name').fill('Forms Mismatch')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill('password123')
  await page.getByLabel('Confirm password').fill('something-else')
  await page.getByRole('button', { name: 'Sign up' }).click()

  await expect(page.locator('.auth-error')).toHaveText('passwords do not match')
  // Never even attempted the API call - still on the register form.
  await expect(page).toHaveURL(/#\/register$/)

  // Fixing the mismatch lets the same form actually succeed.
  await page.getByLabel('Confirm password').fill('password123')
  await page.getByRole('button', { name: 'Sign up' }).click()
  await page.waitForURL(/#\/dashboard$/)
})

test('saving an empty name is blocked client-side and never touches the session', async ({ page }) => {
  const user = await registerNewUser(page, 'Forms Edit')
  const sidebarUser = page.locator('.sidebar-user')

  await page.getByText('View all').click()
  await page.waitForURL(/#\/dashboard\/users\/\d+\/notes$/)
  await page.getByRole('link', { name: '← Back to user' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+$/)

  const nameInput = page.locator('#edit-name')
  await nameInput.fill('')
  await page.getByRole('button', { name: 'Save changes' }).click()

  await expect(nameInput).toHaveJSProperty('validity.valid', false)
  await expect(page.locator('.auth-error')).toHaveCount(0)
  await expect(sidebarUser).toHaveText(user.name)

  await nameInput.fill('Forms Edit Fixed')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(sidebarUser).toHaveText('Forms Edit Fixed')
})

test('creating a note requires a title; a valid submission clears the form and appears immediately', async ({
  page,
}) => {
  await registerNewUser(page, 'Forms Notes')

  await page.getByText('View all').click()
  await page.waitForURL(/#\/dashboard\/users\/\d+\/notes$/)
  await expect(page.getByText('No notes yet.')).toBeVisible()

  // Empty title: handleCreate no-ops, nothing happens, nothing crashes.
  await page.getByRole('button', { name: 'Add note' }).click()
  await expect(page.getByText('No notes yet.')).toBeVisible()

  await page.getByPlaceholder('Title').fill('Forms test note')
  await page.getByPlaceholder('Write something…').fill('body text')
  await page.getByRole('button', { name: 'Add note' }).click()

  await expect(page.locator('.note-card', { hasText: 'Forms test note' })).toBeVisible()
  await expect(page.getByPlaceholder('Title')).toHaveValue('')
  await expect(page.getByPlaceholder('Write something…')).toHaveValue('')
})

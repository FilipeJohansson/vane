import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'
import { sidebarLink } from '../support/nav'

// Covers the delete-confirmation modal (UserDetail.vane + src/components/ui/Modal.vane),
// core.Portal rendering into #modal-root (DashboardShell.vane) rather than inline in the
// page's own DOM subtree.

test('the modal renders through the portal target, not inline, and Cancel leaves no trace', async ({ page }) => {
  const user = await registerNewUser(page, 'Portal User')

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/\/dashboard\/users$/)
  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+$/)

  await expect(page.locator('#modal-root .modal-overlay')).toHaveCount(0)

  await page.getByRole('button', { name: 'Delete account' }).click()

  // It's actually portaled out: present under #modal-root, absent from the
  // routed page's own container.
  await expect(page.locator('#modal-root .modal-overlay')).toBeVisible()
  await expect(page.locator('.page .modal-overlay')).toHaveCount(0)
  await expect(page.locator('.modal-title')).toHaveText('Delete account?')

  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.locator('#modal-root .modal-overlay')).toHaveCount(0)
  // Cancel didn't navigate or delete anything.
  await expect(page).toHaveURL(/\/dashboard\/users\/\d+$/)
  await expect(page.getByRole('button', { name: 'Delete account' })).toBeVisible()
})

test('repeated open/close cycles leave no leftover content in the portal target', async ({ page }) => {
  const user = await registerNewUser(page, 'Portal Cycles')

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/\/dashboard\/users$/)
  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+$/)

  for (let i = 0; i < 3; i++) {
    await page.getByRole('button', { name: 'Delete account' }).click()
    await expect(page.locator('#modal-root .modal-overlay')).toHaveCount(1)
    await page.getByRole('button', { name: 'Cancel' }).click()
    await expect(page.locator('#modal-root .modal-overlay')).toHaveCount(0)
  }
})

test('confirming in the modal actually deletes the account and navigates to login', async ({ page }) => {
  await registerNewUser(page, 'Portal Confirm Delete')

  await page.getByText('View all').click()
  await page.waitForURL(/\/dashboard\/users\/\d+\/notes$/)
  await page.getByRole('link', { name: '← Back to user' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+$/)

  await page.getByRole('button', { name: 'Delete account' }).click()
  await expect(page.locator('#modal-root .modal-overlay')).toBeVisible()

  await page.locator('#modal-root').getByRole('button', { name: 'Confirm' }).click()
  await page.waitForURL(/\/login$/)
})

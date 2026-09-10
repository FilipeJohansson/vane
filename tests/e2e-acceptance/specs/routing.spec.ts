import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'
import { sidebarLink } from '../support/nav'

test('sidebar links navigate the nested dashboard routes and mark the active one', async ({ page }) => {
  const user = await registerNewUser(page, 'Routing Nav')

  await expect(sidebarLink(page, 'Overview')).toHaveClass(/sidebar-link--active/)
  await expect(sidebarLink(page, 'Users')).not.toHaveClass(/sidebar-link--active/)

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/#\/dashboard\/users$/)
  await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()
  await expect(sidebarLink(page, 'Users')).toHaveClass(/sidebar-link--active/)
  await expect(sidebarLink(page, 'Overview')).not.toHaveClass(/sidebar-link--active/)

  // Own row's "View" link (the list may hold other users from earlier
  // specs, since the backend store is shared - never assume it's the only
  // row), params-driven users/:id.
  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+$/)
  await expect(page.getByRole('heading', { name: 'User detail' })).toBeVisible()
  // Nested one level further: users/:id/notes, same :id carried through.
  await expect(sidebarLink(page, 'Overview')).not.toHaveClass(/sidebar-link--active/)

  await page.getByRole('link', { name: 'View notes' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+\/notes$/)
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible()
})

test('the users/:id route resolves per-id: another user is read-only, own page stays editable', async ({
  page,
  browser,
}) => {
  const userA = await registerNewUser(page, 'Routing User A')

  const contextB = await browser.newContext()
  const pageB = await contextB.newPage()
  const userB = await registerNewUser(pageB, 'Routing User B')
  await contextB.close()

  // As A: open B's row from the Users list, by B's own :id.
  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/#\/dashboard\/users$/)
  await page.getByRole('row', { name: new RegExp(userB.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+$/)

  await expect(page.getByText(userB.email)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Delete account' })).toHaveCount(0)
  await expect(page.locator('#edit-name')).toHaveCount(0)

  // Back to the Users list, then A's own row - same route pattern, different
  // :id, resolves to A's own (editable) data instead of B's.
  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/#\/dashboard\/users$/)
  await page.getByRole('row', { name: new RegExp(userA.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+$/)

  await expect(page.getByText(userA.email)).toBeVisible()
  await expect(page.locator('#edit-name')).toHaveValue(userA.name)
  await expect(page.getByRole('button', { name: 'Delete account' })).toBeVisible()
})

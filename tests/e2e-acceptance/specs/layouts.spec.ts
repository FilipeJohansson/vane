import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'
import { sidebarLink } from '../support/nav'

// DashboardShell.vane's own comment states the contract: it persists across
// /dashboard/* sub-navigation, only router.Outlet() content changes. Proving
// that black-box (without touching framework internals): Vane has no VDOM,
// so if DashboardShell's own render never reruns, a marker hand-injected on
// its root node survives untouched across every sub-route change below. If
// the shell were torn down and remounted, a fresh render would wipe it.
test('DashboardShell persists across nested route changes, only the outlet content swaps', async ({ page }) => {
  const user = await registerNewUser(page, 'Layout Persist')

  await page.evaluate(() => {
    document.querySelector('.shell')?.setAttribute('data-persist-probe', 'untouched')
  })
  const shellPersisted = () => expect(page.locator('.shell')).toHaveAttribute('data-persist-probe', 'untouched')

  await expect(page.getByRole('heading', { name: /^Welcome, Layout Persist$/ })).toBeVisible()
  await shellPersisted()

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/\/dashboard\/users$/)
  await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()
  await shellPersisted()

  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+$/)
  await expect(page.getByRole('heading', { name: 'User detail' })).toBeVisible()
  await shellPersisted()

  await page.getByRole('link', { name: 'View notes' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+\/notes$/)
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible()
  await shellPersisted()

  await sidebarLink(page, 'Overview').click()
  await page.waitForURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: /^Welcome, Layout Persist$/ })).toBeVisible()
  await shellPersisted()
})

// A simpler, content-level companion to the DOM-persistence test above: the
// sidebar's own user display never flickers/resets while navigating within
// /dashboard/*, which is what an end user would actually notice if the shell
// were being torn down and rebuilt on every sub-route change.
test('the sidebar keeps showing the same signed-in user across sub-routes', async ({ page }) => {
  const user = await registerNewUser(page, 'Layout User')

  const sidebarUser = page.locator('.sidebar-user')
  await expect(sidebarUser).toHaveText(user.name)

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/\/dashboard\/users$/)
  await expect(sidebarUser).toHaveText(user.name)

  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/\/dashboard\/users\/\d+$/)
  await expect(sidebarUser).toHaveText(user.name)
})

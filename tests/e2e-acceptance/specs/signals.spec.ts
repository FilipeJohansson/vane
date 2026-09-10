import { expect, test } from '@playwright/test'
import { registerNewUser } from '../support/auth'
import { sidebarLink } from '../support/nav'

// Two signals are in play on UserDetail.vane: a local `name` (the form's own
// state, bound to the "Name" input) and the shared `store.CurrentUser` (the
// session, read by the sidebar and Dashboard's heading). This spec proves
// both react correctly, and - just as importantly - that they stay isolated
// from each other until the form actually commits.
test('editing the name field stays local until saved, then propagates through the shared session signal', async ({
  page,
}) => {
  const user = await registerNewUser(page, 'Signals Original')
  const sidebarUser = page.locator('.sidebar-user')
  const newName = 'Signals Updated'

  await sidebarLink(page, 'Users').click()
  await page.waitForURL(/#\/dashboard\/users$/)
  await page.getByRole('row', { name: new RegExp(user.email) }).getByRole('link', { name: 'View' }).click()
  await page.waitForURL(/#\/dashboard\/users\/\d+$/)

  const nameInput = page.locator('#edit-name')
  await expect(nameInput).toHaveValue(user.name)

  // Typing updates the form's own local signal (reflected back into the
  // controlled input) without touching the shared session signal at all.
  await nameInput.fill(newName)
  await expect(nameInput).toHaveValue(newName)
  await expect(sidebarUser).toHaveText(user.name)

  // Saving commits the local value into store.CurrentUser - every reader of
  // that shared signal updates immediately, no refetch or navigation needed.
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(sidebarUser).toHaveText(newName)

  // Still true after actually navigating elsewhere: it's the same signal,
  // not a value that happened to be passed down to the sidebar once.
  await sidebarLink(page, 'Overview').click()
  await page.waitForURL(/#\/dashboard$/)
  await expect(page.getByRole('heading', { name: new RegExp(`^Welcome, ${newName}$`) })).toBeVisible()
  await expect(sidebarUser).toHaveText(newName)
})

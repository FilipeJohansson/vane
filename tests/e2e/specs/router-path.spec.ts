import { expect, test } from '@playwright/test'

// A pathname under /path-mode boots the shared fixture with PathLocation
// instead of the default HashLocation (see App.vane's pathModeBase gate),
// so these specs run against the same app/routes as router.spec.ts, just
// addressed with real paths instead of a hash fragment. router.spec.ts's
// hash-based specs are left untouched as the regression baseline.

test('navigates between fixture pages using real paths, no hash', async ({ page }) => {
  await page.goto('/path-mode/smoke')

  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page).toHaveURL(/\/path-mode\/smoke$/)
  await expect(page).not.toHaveURL(/#/)

  await page.getByRole('link', { name: 'Signals' }).click()

  await expect(page).toHaveURL(/\/path-mode\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('a modified click opens a new tab instead of navigating in place', async ({ page, context }) => {
  await page.goto('/path-mode/smoke')

  const [popup] = await Promise.all([
    context.waitForEvent('page'),
    page.getByRole('link', { name: 'Signals' }).click({ modifiers: ['ControlOrMeta'] }),
  ])
  await popup.waitForLoadState()

  expect(popup.url()).toContain('/path-mode/signals')
  await expect(page).toHaveURL(/\/path-mode\/smoke$/)
})

test('a middle click does not navigate the current page', async ({ page, browserName }) => {
  // Playwright's WebKit driver dispatches a middle click as button: 0 (a
  // regular left click) instead of button: 1, unlike Chromium/Firefox -
  // a driver quirk, not real Safari behavior (which, like every other
  // browser, fires "auxclick" for the middle button, never "click", so
  // PathLocation's listener wouldn't even see it). The button check itself
  // is already covered directly and reliably by a unit test
  // (TestPathLocationHandleClick/middle_click, core/router/path_location_dom_test.go).
  test.skip(browserName === 'webkit', "WebKit's Playwright driver simulates a middle click as button: 0")

  await page.goto('/path-mode/smoke')

  await page.getByRole('link', { name: 'Signals' }).click({ button: 'middle' })

  await expect(page).toHaveURL(/\/path-mode\/smoke$/)
})

test('supports browser back and forward with real paths', async ({ page }) => {
  await page.goto('/path-mode/smoke')
  await page.getByRole('link', { name: 'Signals' }).click()
  await expect(page).toHaveURL(/\/path-mode\/signals$/)

  await page.goBack()
  await expect(page).toHaveURL(/\/path-mode\/smoke$/)
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goForward()
  await expect(page).toHaveURL(/\/path-mode\/signals$/)
  await expect(page.getByTestId('signal-count')).toHaveText('0')
})

test('supports a direct load and a refresh of a nested route', async ({ page }) => {
  await page.goto('/path-mode/users/42')
  await expect(page.getByTestId('user-id')).toHaveText('42')

  await page.reload()

  await expect(page).toHaveURL(/\/path-mode\/users\/42$/)
  await expect(page.getByTestId('user-id')).toHaveText('42')
})

test('a navigation with a fragment scrolls to that element instead of the page top', async ({ page }) => {
  await page.goto('/path-mode/smoke')
  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page.getByTestId('target-section')).not.toBeInViewport()

  await page.getByTestId('scroll-to-anchor').click()

  await expect(page).toHaveURL(/\/path-mode\/smoke#target-section$/)
  await expect(page.getByTestId('target-section')).toBeInViewport()
})

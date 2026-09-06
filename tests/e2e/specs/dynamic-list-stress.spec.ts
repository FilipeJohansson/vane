import { expect, test } from '@playwright/test'

test('dynamic list churn disposes every item exactly once', async ({ page }) => {
  await page.goto('/#/lifecycle')

  const created = page.getByTestId('list-items-created')
  const cleaned = page.getByTestId('list-items-cleaned')

  await expect(created).toHaveText('0')
  await expect(cleaned).toHaveText('0')

  await page.getByRole('button', { name: 'Populate list' }).click()
  await expect(created).toHaveText('200')
  await expect(cleaned).toHaveText('0')
  await expect(page.getByTestId('stress-list').locator('li')).toHaveCount(200)

  await page.getByRole('button', { name: 'Clear list' }).click()
  await expect(cleaned).toHaveText('200')
  await expect(page.getByTestId('stress-list').locator('li')).toHaveCount(0)

  // Re-populating replaces the whole set with fresh items/effects; the
  // previous (already-cleared) generation must not be double-counted, and
  // the new one must not start out already counted as cleaned.
  await page.getByRole('button', { name: 'Populate list' }).click()
  await expect(created).toHaveText('400')
  await expect(cleaned).toHaveText('200')

  await page.getByRole('button', { name: 'Clear list' }).click()
  await expect(cleaned).toHaveText('400')
})

test('leaving the route disposes a populated but never-cleared list', async ({ page }) => {
  await page.goto('/#/lifecycle')

  await page.getByRole('button', { name: 'Populate list' }).click()
  await expect(page.getByTestId('list-items-created')).toHaveText('200')
  await expect(page.getByTestId('list-items-cleaned')).toHaveText('0')

  await page.goto('/#/smoke')
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('list-items-cleaned')).toHaveText('200')
})

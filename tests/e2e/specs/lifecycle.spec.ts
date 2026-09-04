import { expect, test } from '@playwright/test'

test('runs effects on mount and when dependencies change', async ({ page }) => {
  await page.goto('/#/lifecycle')

  const effectRuns = page.getByTestId('effect-runs')
  await expect(effectRuns).toHaveText('1')

  await page.getByRole('button', { name: 'Run effect' }).click()
  await expect(effectRuns).toHaveText('2')
})

test('runs component cleanup when leaving the route', async ({ page }) => {
  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('lifecycle-cleanups')).toHaveText('0')

  await page.goto('/#/smoke')
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('lifecycle-cleanups')).toHaveText('1')
})

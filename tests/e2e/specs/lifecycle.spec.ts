import { expect, test } from '@playwright/test'

test('runs effects on mount and when dependencies change', async ({ page }) => {
  await page.goto('/#/lifecycle')

  const effectRuns = page.getByTestId('effect-runs')
  await expect(effectRuns).toHaveText('1')

  await page.getByRole('button', { name: 'Run effect' }).click()
  await expect(effectRuns).toHaveText('2')
  await expect(page.getByTestId('effect-cleanups')).toHaveText('1')
})

test('runs component cleanup when leaving the route', async ({ page }) => {
  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('lifecycle-cleanups')).toHaveText('0')

  await page.goto('/#/smoke')
  await expect(page.getByTestId('app-ready')).toHaveText('ready')

  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('lifecycle-cleanups')).toHaveText('1')
})

test('disposes window listeners when leaving the route', async ({ page }) => {
  await page.goto('/#/lifecycle')
  await expect(page.getByTestId('effect-runs')).toHaveText('1')
  await page.evaluate(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' })))
  await expect(page.getByTestId('listener-calls')).toHaveText('1')

  await page.goto('/#/smoke')
  await page.evaluate(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'b' })))
  await page.goto('/#/lifecycle')

  await expect(page.getByTestId('listener-calls')).toHaveText('1')
  await expect(page.getByTestId('listener-cleanups')).toHaveText('1')
  await expect(page.getByTestId('unmount-probe')).toHaveText('1')
})

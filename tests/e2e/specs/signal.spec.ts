import { expect, test } from '@playwright/test'

test('updates the DOM when a signal changes', async ({ page }) => {
  await page.goto('/')

  const count = page.getByTestId('signal-count')

  await expect(count).toHaveText('0')

  await page.getByRole('button', { name: '+' }).click()
  await page.getByRole('button', { name: '+' }).click()
  await expect(count).toHaveText('2')

  await page.getByRole('button', { name: '-' }).click()
  await expect(count).toHaveText('1')
})

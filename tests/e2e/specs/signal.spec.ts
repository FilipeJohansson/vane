import { expect, test } from '@playwright/test'

test('updates the DOM when a signal changes', async ({ page }) => {
  await page.goto('/#/signals')

  const count = page.getByTestId('signal-count')

  await expect(count).toHaveText('0')

  await page.getByRole('button', { name: '+' }).click()
  await page.getByRole('button', { name: '+' }).click()
  await expect(count).toHaveText('2')

  await page.getByRole('button', { name: '-' }).click()
  await expect(count).toHaveText('1')
})

test('updates computed values and tracks multiple dependencies', async ({ page }) => {
  await page.goto('/#/signals')

  await expect(page.getByTestId('computed-double')).toHaveText('0')
  await expect(page.getByTestId('multiple-dependencies')).toHaveText('10')

  await page.getByRole('button', { name: '+' }).click()
  await expect(page.getByTestId('computed-double')).toHaveText('2')
  await expect(page.getByTestId('multiple-dependencies')).toHaveText('11')

  await page.getByRole('button', { name: 'Add dependency' }).click()
  await expect(page.getByTestId('multiple-dependencies')).toHaveText('12')
})

import { expect, test } from '@playwright/test'

test('renders the synchronous panic fallback', async ({ page }) => {
  await page.goto('/#/errors')

  await expect(page.getByTestId('sync-fallback')).toHaveText('sync failure')
  await expect(page.getByTestId('outside-content')).toHaveText('outside')
})

test('recovers reactive panics while content outside the boundary keeps working', async ({ page }) => {
  await page.goto('/#/errors')

  await expect(page.getByTestId('recovered-value')).toHaveText('healthy')
  await page.getByRole('button', { name: 'Toggle reactive panic' }).click()
  await expect(page.getByTestId('reactive-fallback')).toHaveText('reactive failure')

  await page.getByRole('button', { name: 'Update outside' }).click()
  await expect(page.getByTestId('outside-count')).toHaveText('1')

  await page.getByRole('button', { name: 'Toggle reactive panic' }).click()
  await expect(page.getByTestId('recovered-value')).toHaveText('healthy')
})

test('disposes an unguarded reactive binding after panic', async ({ page }) => {
  await page.goto('/#/errors')

  await expect(page.getByTestId('inner-binding')).toHaveText('healthy')
  await page.getByRole('button', { name: 'Toggle inner panic' }).click()
  await expect(page.getByTestId('inner-binding')).toHaveText('healthy')

  await page.getByRole('button', { name: 'Update outside' }).click()
  await expect(page.getByTestId('outside-count')).toHaveText('1')
})
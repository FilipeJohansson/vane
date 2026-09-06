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

test('many panic/recover cycles never leave stale content or a dead boundary behind', async ({ page }) => {
  await page.goto('/#/errors')

  const boundary = page.getByTestId('reactive-boundary')
  const toggle = page.getByRole('button', { name: 'Toggle reactive panic' })

  const cycles = 25
  for (let i = 0; i < cycles; i++) {
    await toggle.click() // healthy -> broken
    await expect(page.getByTestId('reactive-fallback')).toHaveText('reactive failure')
    // The boundary must hold exactly the fallback, never the fallback
    // stacked on top of a leftover healthy node from before the panic.
    await expect(boundary.locator('> *')).toHaveCount(1)

    await toggle.click() // broken -> healthy
    await expect(page.getByTestId('recovered-value')).toHaveText('healthy')
    await expect(boundary.locator('> *')).toHaveCount(1)
  }

  // A fresh click listener from the LAST recovery must still be the one
  // responding - not a zombie from an earlier cycle double- or under-firing.
  await page.getByTestId('healthy-click').click()
  await expect(page.getByTestId('healthy-clicks')).toHaveText('1')

  // Content entirely outside the boundary was never touched by any of this.
  await page.getByRole('button', { name: 'Update outside' }).click()
  await expect(page.getByTestId('outside-count')).toHaveText('1')
})
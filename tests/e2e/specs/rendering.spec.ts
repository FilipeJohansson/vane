import { expect, test } from '@playwright/test'

test('renders elements, attributes, and nested components', async ({ page }) => {
  await page.goto('/#/rendering')

  await expect(page.getByTestId('rendering-heading')).toHaveText('Rendering')
  await expect(page.getByTestId('nested-child')).toHaveText('nested child')
})

test('renders fragments without an extra wrapper and toggles conditional content', async ({ page }) => {
  await page.goto('/#/rendering')

  await expect(page.getByTestId('fragment').locator(':scope > *')).toHaveCount(2)
  await expect(page.getByTestId('conditional-content')).toHaveCount(0)

  await page.getByRole('button', { name: 'Toggle details' }).click()
  await expect(page.getByTestId('conditional-content')).toHaveText('details visible')

  await page.getByRole('button', { name: 'Toggle details' }).click()
  await expect(page.getByTestId('conditional-content')).toHaveCount(0)
})

test('adds and removes dynamic children', async ({ page }) => {
  await page.goto('/#/rendering')

  const items = page.getByTestId('dynamic-item')
  await expect(items).toHaveCount(2)

  await page.getByRole('button', { name: 'Add item' }).click()
  await expect(items).toHaveCount(3)
  await expect(items.nth(2)).toHaveText('third')

  await page.getByRole('button', { name: 'Remove item' }).click()
  await expect(items).toHaveCount(2)
})
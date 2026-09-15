import { expect, test } from '@playwright/test'

test('renders elements, attributes, and nested components', async ({ page }) => {
  await page.goto('/rendering')

  await expect(page.getByTestId('rendering-heading')).toHaveText('Rendering')
  await expect(page.getByTestId('nested-child')).toHaveText('nested child')
})

test('renders fragments without an extra wrapper and toggles conditional content', async ({ page }) => {
  await page.goto('/rendering')

  await expect(page.getByTestId('fragment').locator(':scope > *')).toHaveCount(2)
  await expect(page.getByTestId('conditional-content')).toHaveCount(0)

  await page.getByRole('button', { name: 'Toggle details' }).click()
  await expect(page.getByTestId('conditional-content')).toHaveText('details visible')

  await page.getByRole('button', { name: 'Toggle details' }).click()
  await expect(page.getByTestId('conditional-content')).toHaveCount(0)
})

test('adds and removes dynamic children', async ({ page }) => {
  await page.goto('/rendering')

  const items = page.getByTestId('dynamic-item')
  await expect(items).toHaveCount(2)

  await page.getByRole('button', { name: 'Add item' }).click()
  await expect(items).toHaveCount(3)
  await expect(items.nth(2)).toHaveText('third')

  await page.getByRole('button', { name: 'Remove item' }).click()
  await expect(items).toHaveCount(2)
})

// Real, real-browser coverage of a keyed {for} whose body renders 2 sibling
// nodes per item (a <dt>/<dd> pair) - the one-key-to-many-nodes DynList
// shape, going through the actual .vane -> compat-compile -> hint-resolve ->
// promoted-compile pipeline `vane run` uses, not just a unit test.
test('keyed list with 2 nodes per item mounts both and updates only the changed item', async ({ page }) => {
  await page.goto('/rendering')

  const terms = page.getByTestId('keyed-term')
  const defs = page.getByTestId('keyed-def')
  await expect(terms).toHaveCount(2)
  await expect(defs).toHaveCount(2)
  await expect(terms.nth(0)).toHaveText('Alpha')
  await expect(defs.nth(0)).toHaveText('First letter')
  await expect(terms.nth(1)).toHaveText('Beta')
  await expect(defs.nth(1)).toHaveText('Second letter')

  await page.getByRole('button', { name: 'Update first definition' }).click()

  await expect(defs.nth(0)).toHaveText('Updated definition')
  // The second entry's own pair must be untouched by the first entry's update.
  await expect(terms.nth(1)).toHaveText('Beta')
  await expect(defs.nth(1)).toHaveText('Second letter')
  await expect(terms).toHaveCount(2)
  await expect(defs).toHaveCount(2)
})
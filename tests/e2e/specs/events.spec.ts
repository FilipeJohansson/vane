import { expect, test } from '@playwright/test'

test('updates a signal from browser input events', async ({ page }) => {
  await page.goto('/#/events')

  const input = page.getByLabel('Name')
  const output = page.getByTestId('input-output')

  await expect(output).toHaveText('')

  await input.fill('Ada')
  await expect(output).toHaveText('Ada')

  await input.fill('Ada Lovelace')
  await expect(output).toHaveText('Ada Lovelace')
})

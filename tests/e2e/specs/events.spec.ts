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

  const keyboardInput = page.getByLabel('Keyboard input')
  await keyboardInput.press('Enter')
  await expect(page.getByTestId('last-key')).toHaveText('Enter')

  await page.getByRole('button', { name: 'Submit' }).click()
  await expect(page.getByTestId('submit-status')).toHaveText('submitted')
})

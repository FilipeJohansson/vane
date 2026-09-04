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

test('handles click, change, and keyup events', async ({ page }) => {
  await page.goto('/#/events')

  await page.getByRole('button', { name: 'Click' }).click()
  await expect(page.getByTestId('click-count')).toHaveText('1')

  await page.getByLabel('Choice').selectOption('two')
  await expect(page.getByTestId('change-output')).toHaveText('two')

  await page.getByLabel('Keyup input').press('Escape')
  await expect(page.getByTestId('last-key-up')).toHaveText('Escape')
})

test('prevents default navigation and controls propagation', async ({ page }) => {
  await page.goto('/#/events')

  await page.getByRole('link', { name: 'Prevent navigation' }).click()
  await expect(page.getByTestId('prevented')).toHaveText('yes')
  await expect(page).toHaveURL(/#\/events$/)

  await page.getByRole('button', { name: 'Bubble', exact: true }).click()
  await expect(page.getByTestId('parent-clicks')).toHaveText('1')

  await page.getByRole('button', { name: 'Stop bubble' }).click()
  await expect(page.getByTestId('stopped-parent-clicks')).toHaveText('0')
})

import { expect, test } from '@playwright/test'

test('mounts portal content in an external host and cleans it up', async ({ page }) => {
  await page.goto('/#/portals')

  await expect(page.getByTestId('portal-source')).toHaveText('source')
  await expect(page.getByTestId('portal-host')).toBeEmpty()

  await page.getByRole('button', { name: 'Open portal' }).click()
  await expect(page.getByTestId('portal-host')).toContainText('mounted outside route')
  await expect(page.getByTestId('portal-content')).toBeVisible()

  await page.getByRole('button', { name: 'Close portal' }).click()
  await expect(page.getByTestId('portal-host')).toBeEmpty()

  await page.getByRole('button', { name: 'Open portal' }).click()
  await expect(page.getByTestId('portal-content')).toBeVisible()

  await page.goto('/#/smoke')

  await expect(page.getByTestId('app-ready')).toHaveText('ready')
  await expect(page.getByTestId('portal-host')).toBeEmpty()
})

test('repeated open/close cycles do not leak portal content', async ({ page }) => {
  await page.goto('/#/portals')

  const opens = page.getByTestId('portal-opens')
  const closes = page.getByTestId('portal-closes')
  await expect(opens).toHaveText('0')
  await expect(closes).toHaveText('0')

  const cycles = 20
  for (let i = 0; i < cycles; i++) {
    await page.getByRole('button', { name: 'Open portal' }).click()
    await expect(page.getByTestId('portal-content')).toBeVisible()
    await expect(opens).toHaveText(String(i + 1))

    await page.getByRole('button', { name: 'Close portal' }).click()
    await expect(page.getByTestId('portal-host')).toBeEmpty()
    await expect(closes).toHaveText(String(i + 1))
  }
})

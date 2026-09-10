import { expect, test } from '@playwright/test'

test('boots the fullstack example app and reaches the real API', async ({ page }) => {
  await page.goto('/#/login')

  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible()
})

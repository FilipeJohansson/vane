import type { Page } from '@playwright/test'

// UserDetail.vane always renders a "← Back to Users" link, so a bare
// getByRole('link', { name: 'Users' }) is ambiguous once that's on screen
// (Playwright's default name match is substring) - scope sidebar nav lookups
// to the <nav> element instead. Shared since routing/layouts specs both need it.
export function sidebarLink(page: Page, name: string) {
  return page.getByRole('navigation').getByRole('link', { name })
}

import { test, expect, login } from './fixtures'

test('session survives reload; logout protects pages; wrong password allows recovery', async ({ page }) => {
  await page.reload()
  await expect(page.getByRole('button', { name: 'Sign out', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await expect(page).toHaveURL(/\/login$/)
  await page.goto('/server')
  await expect(page).toHaveURL(/\/login$/)
  await login(page, 'deliberately-wrong-password')
  await expect(page.locator('.n-notification')).toContainText('Connection failed')
  await expect(page).toHaveURL(/\/login$/)
  await login(page)
  await expect(page).toHaveURL(/\/server$/)
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem('dbm-last-login') ?? '{}'))
  expect(stored).not.toHaveProperty('password')
})

import { expect, test } from '@playwright/test'

test('login header settings apply immediately and survive a reload', async ({ page }) => {
  await page.goto('/login')
  await page.locator('#login-host').fill('db.example')
  const settings = page.getByRole('banner').getByRole('button', { name: 'Настройки' })
  await settings.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('radio', { name: 'English' }).check()
  await expect(dialog).toHaveAccessibleName('Settings')
  await dialog.getByRole('radio', { name: 'Dark' }).check()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Settings' })).toBeFocused()
  await expect(page.locator('#login-host')).toHaveValue('db.example')

  await page.reload()
  await page.getByRole('button', { name: 'Settings' }).click()
  await expect(dialog.getByRole('radio', { name: 'English' })).toBeChecked()
  await expect(dialog.getByRole('radio', { name: 'Dark' })).toBeChecked()
})

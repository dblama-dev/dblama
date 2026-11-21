import { randomUUID } from 'node:crypto'
import { test as base, expect, type Locator, type Page } from '@playwright/test'
export { expect }
export type Engine = 'mysql' | 'mariadb' | 'postgres'
const engine = (process.env.E2E_ENGINE ?? 'mysql') as Engine
const database = process.env.E2E_DB_NAME ?? 'dbmanager_e2e'

export function dbPath(db: string, suffix = ''): string {
  return `/db/${encodeURIComponent(db)}${suffix ? '/' + suffix.replace(/^\//, '') : ''}`
}
export function tablePath(db: string, table: string, view = 'data'): string {
  return `${dbPath(db, `table/${encodeURIComponent(table)}`)}?view=${view}`
}
export async function selectOption(page: Page, control: Locator, text: string): Promise<void> {
  const select = control.locator('select')
  if (await select.count()) {
    await select.selectOption({ label: text })
    return
  }
  await control.click()
  const search = control.locator('input')
  if (await search.count()) await search.fill(text)
  await page.locator('.ui-select__option:visible').filter({ has: page.getByText(text, { exact: true }) }).click()
  await page.keyboard.press('Escape')
  await expect(page.locator('.ui-select__list:visible')).toHaveCount(0)
}
export async function login(page: Page, password = process.env.E2E_DB_PASSWORD ?? 'e2e-password', dbName = database): Promise<void> {
  await page.goto('/login')
  const form = page.locator('.login-view')
  await selectOption(page, form.locator('.ui-select'), { mysql: 'MySQL', mariadb: 'MariaDB', postgres: 'PostgreSQL' }[engine])
  const inputs = form.locator('input.ui-input__control')
  // UiSelect's search input is not an UiInput; these are the five text connection fields.
  await inputs.nth(0).fill(process.env.E2E_DB_HOST ?? '127.0.0.1')
  await inputs.nth(1).fill(process.env.E2E_DB_PORT!)
  await inputs.nth(2).fill(process.env.E2E_DB_USER ?? (engine === 'postgres' ? 'postgres' : 'root'))
  await inputs.nth(3).fill(password)
  await inputs.nth(4).fill(dbName)
  await form.getByRole('button', { name: 'Connect', exact: true }).click()
}
export async function runSQL(page: Page, sql: string): Promise<void> {
  const match = new URL(page.url()).pathname.match(/^\/db\/([^/]+)/)
  const db = match ? decodeURIComponent(match[1]!) : database
  await page.goto(dbPath(db, 'console'))
  // Replace a restored draft through CodeMirror's selection and input handlers.
  const editor = page.locator('.cm-content')
  await editor.press('ControlOrMeta+a')
  await page.keyboard.insertText(sql)
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith('/query'))
  await page.getByRole('button', { name: 'Execute all', exact: true }).click()
  const result = await response
  expect(result.ok(), await result.text()).toBe(true)
  await expect(page.getByRole('button', { name: 'Execute all', exact: true })).toBeEnabled()
  await expect(page.locator('.console__error')).toHaveCount(0)
}

export const test = base.extend<{ db: string; engine: Engine; prefix: string }>({
  db: async ({}, use) => { await use(database) },
  engine: async ({}, use) => { await use(engine) },
  prefix: async ({}, use) => { await use(`e2e_${randomUUID().replaceAll('-', '').slice(0, 12)}`) },
  page: async ({ page }, use) => {
    await page.addInitScript(() => localStorage.setItem('dbm-locale', 'en'))
    await login(page)
    await expect(page).toHaveURL(/\/server$/)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    try {
      await use(page)
      expect(errors, 'uncaught browser errors').toEqual([])
    } finally {
      await page.request.delete('/api/session', { headers: { 'X-Requested-With': 'XMLHttpRequest' } })
    }
  },
})

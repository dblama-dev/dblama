import { randomUUID } from 'node:crypto'
import type { Browser, Page } from '@playwright/test'
import { expect, runSQL, test, type Engine } from './fixtures'

const headers = { 'X-Requested-With': 'XMLHttpRequest' }

async function openAccount(page: Page, username: string) {
  await page.goto('/server/users')
  await page.locator('[data-test=find-account]').fill(username)
  await page.locator('[data-test=select-user]').click()
  return page.locator('.grant-workspace')
}

async function checkReads(browser: Browser, origin: string, engine: Engine, database: string,
  user: string, password: string, allowed: string[], denied: string[]) {
  const context = await browser.newContext({ baseURL: origin })
  try {
    const login = await context.request.post('/api/session', { headers, data: {
      driver: engine, host: process.env.E2E_DB_HOST ?? '127.0.0.1', port: process.env.E2E_DB_PORT, user, password, database,
    } })
    expect(login.ok(), await login.text()).toBe(true)
    const read = (table: string) => context.request.post('/api/databases/' + database + '/query', {
      headers, data: { sql: 'SELECT * FROM ' + table, queryID: randomUUID() },
    })
    for (const table of allowed) {
      const res = await read(table)
      expect(res.ok(), await res.text()).toBe(true)
    }
    for (const table of denied) {
      const res = await read(table)
      expect(res.ok()).toBe(false)
      expect(await res.text()).toMatch(/denied/i)
    }
  } finally {
    await context.request.delete('/api/session', { headers })
    await context.close()
  }
}

test('database prefix grant applies to future databases and can be revoked in the editor', async ({ page, browser, engine, prefix }) => {
  test.skip(engine === 'postgres', 'MySQL/MariaDB grant patterns')
  const password = randomUUID()
  const matching = prefix + '_reports'
  const adjacent = prefix + 'Xreports'
  const account = "'" + prefix + "'@'%'"
  await runSQL(page, 'CREATE USER ' + account + " IDENTIFIED BY '" + password + "'")
  try {
    const editor = await openAccount(page, prefix)
    await editor.locator('[data-test=choose-scope]').click()
    await editor.locator('[data-test=username-database-pattern]').click()
    const pattern = prefix.replaceAll('_', '\\_') + '\\_%'
    await expect(editor.locator('[data-test=scope-database-pattern]')).toHaveValue(pattern)
    await editor.locator('[data-test=open-new-scope]').click()
    await editor.locator('[data-test=privilege-SELECT]').check()
    await editor.locator('[data-test=save-grants]').click()
    const assigned = editor.locator('[data-test=open-scope]').filter({ hasText: pattern })
    await expect(assigned).toContainText('SELECT')
    const origin = new URL(page.url()).origin
    await runSQL(page, 'CREATE DATABASE ' + matching + '; CREATE DATABASE ' + adjacent +
      '; CREATE TABLE ' + matching + '.items (id INT); CREATE TABLE ' + adjacent + '.items (id INT)')
    await checkReads(browser, origin, engine, matching, prefix, password, [matching + '.items'], [adjacent + '.items'])
    await openAccount(page, prefix)
    await assigned.click()
    await editor.locator('[data-test=privilege-SELECT]').uncheck()
    await editor.locator('[data-test=save-grants]').click()
    await expect(assigned).toHaveCount(0)
  } finally {
    await runSQL(page, 'DROP USER IF EXISTS ' + account + '; DROP DATABASE IF EXISTS ' + matching + '; DROP DATABASE IF EXISTS ' + adjacent)
  }
})

test('table mask grants only the existing matching tables of the selected database', async ({ page, browser, db, engine, prefix }) => {
  test.skip(engine === 'postgres', 'MySQL/MariaDB table grants')
  const password = randomUUID()
  const account = "'" + prefix + "'@'%'"
  const first = prefix + '_open', second = prefix + '_closed', adjacent = prefix + 'Xopen', later = prefix + '_later'
  const names = [first, second, adjacent]
  await runSQL(page, 'CREATE USER ' + account + " IDENTIFIED BY '" + password + "'")
  try {
    await runSQL(page, names.map(name => 'CREATE TABLE ' + name + ' (id INT)').join('; '))
    const editor = await openAccount(page, prefix)
    await editor.locator('[data-test=choose-scope]').click()
    await editor.locator('[data-test=scope-level]').selectOption('tablePattern')
    await editor.locator('[data-test=scope-database]').fill(db)
    await editor.locator('[data-test=open-new-scope]').click()
    await editor.locator('[data-test=table-prefix]').click()
    const targets = editor.locator('[data-test=table-matches] li')
    await expect(targets).toHaveCount(2)
    await expect(targets.filter({ hasText: first })).toHaveCount(1)
    await expect(targets.filter({ hasText: second })).toHaveCount(1)
    await editor.locator('[data-test=table-privilege-SELECT]').check()
    await editor.locator('[data-test=apply-table-grants]').click()
    await expect(editor.locator('[data-test=table-grant-result]')).toBeVisible()
    await expect(editor.locator('[data-test=open-scope]').filter({ hasText: second })).toContainText('SELECT')
    const origin = new URL(page.url()).origin
    await runSQL(page, 'CREATE TABLE ' + later + ' (id INT)')
    await checkReads(browser, origin, engine, db, prefix, password, [first, second], [adjacent, later])
  } finally {
    await runSQL(page, 'DROP USER IF EXISTS ' + account + '; DROP TABLE IF EXISTS ' + [...names, later].join(', '))
  }
})

test('all privileges on a custom database pattern can be reopened and narrowed', async ({ page, engine, prefix }) => {
  test.skip(engine === 'postgres', 'MySQL/MariaDB grant patterns')
  const account = "'" + prefix + "'@'%'"
  const pattern = 'reports\\_' + prefix.replaceAll('_', '\\_') + '%'
  await runSQL(page, 'CREATE USER ' + account + ' ACCOUNT LOCK')
  try {
    const editor = await openAccount(page, prefix)
    const assigned = editor.locator('[data-test=open-scope]').filter({ hasText: pattern })
    await editor.locator('[data-test=choose-scope]').click()
    await editor.locator('[data-test=scope-level]').selectOption('databasePattern')
    await editor.locator('[data-test=scope-database-pattern]').fill(pattern)
    await editor.locator('[data-test=open-new-scope]').click()
    await editor.locator('[data-test=toggle-database-pattern-privileges]').check()
    await expect(editor.locator('[data-test=grant-option]')).not.toBeChecked()
    await expect(editor.locator('[data-test=pending-grants]')).toContainText('DELETE')
    await editor.locator('[data-test=save-grants]').click()
    await expect(assigned).toContainText('SELECT')
    await openAccount(page, prefix)
    await assigned.click()
    await expect(editor.locator('[data-test=toggle-database-pattern-privileges]')).toBeChecked()
    await editor.locator('[data-test=privilege-DELETE]').uncheck()
    await editor.locator('[data-test=save-grants]').click()
    await expect(assigned.locator('.scope-description')).not.toContainText(/(?:^|, )DELETE(?:,|$)/)
    await expect(editor.locator('[data-test=privilege-SELECT]')).toBeChecked()
    if (engine === 'mariadb') await expect(editor.locator('[data-test="privilege-DELETE HISTORY"]')).toBeChecked()
  } finally {
    await runSQL(page, 'DROP USER IF EXISTS ' + account)
  }
})

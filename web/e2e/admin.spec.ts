import type { Page } from '@playwright/test'
import { dbPath, expect, login, runSQL, test } from './fixtures'

/** Remove only this test's generated databases, including after an interrupted rename. */
async function cleanupDatabases(page: Page, prefix: string, names: string[]): Promise<void> {
  const response = await page.request.get('/api/databases', {
    headers: { 'X-Requested-With': 'XMLHttpRequest' },
  })
  expect(response.ok(), await response.text()).toBeTruthy()
  const payload = await response.json() as { databases: { name: string }[] }
  for (const name of names) {
    expect(name.startsWith('e2e_') && name.startsWith(prefix)).toBeTruthy()
    if (!payload.databases.some(database => database.name === name)) continue
    const dropped = await page.request.delete(`/api/databases/${encodeURIComponent(name)}`, {
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    })
    expect(dropped.ok(), await dropped.text()).toBeTruthy()
  }
}

test('database create, rename with data, and exact-name drop confirmation', async ({ page, db, engine, prefix }) => {
  const original = `${prefix}admin`
  const renamed = `${prefix}renamed`
  const table = `${prefix}records`
  let connectedDatabase = db
  // PostgreSQL sessions bind to one database; reconnect explicitly for data access.
  async function reconnect(database: string): Promise<void> {
    if (engine !== 'postgres' || connectedDatabase === database) return
    await page.getByRole('button', { name: 'Sign out', exact: true }).click()
    await expect(page).toHaveURL(/\/login$/)
    await login(page, undefined, database)
    await expect(page).toHaveURL(/\/server$/)
    connectedDatabase = database
  }
  try {
    await page.goto('/server')
    const server = page.locator('.server-view')
    await server.getByRole('button', { name: 'Create database', exact: true }).click()
    await server.getByLabel('Database name', { exact: true }).fill(original)
    await server.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(server.locator('.database-form')).toHaveCount(0)
    await server.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect(server.getByRole('cell', { name: original, exact: true })).toHaveCount(0)

    await server.getByRole('button', { name: 'Create database', exact: true }).click()
    await server.getByLabel('Database name', { exact: true }).fill(original)
    await server.getByRole('button', { name: 'Create', exact: true }).click()
    await expect(server.getByRole('cell', { name: original, exact: true })).toBeVisible()
    await server.getByRole('cell', { name: original, exact: true }).click()
    await expect(page.locator('.db-view').getByRole('heading', { name: original, exact: true })).toBeVisible()
    await reconnect(original)
    await page.goto(dbPath(original))
    await runSQL(page, `CREATE TABLE ${table} (id INTEGER PRIMARY KEY, label VARCHAR(80)); INSERT INTO ${table} (id, label) VALUES (7, 'preserved after rename');`)

    await reconnect(db)
    await page.goto(dbPath(original))
    const database = page.locator('.db-view')
    const operations = database.getByRole('button', { name:'Database actions', exact:true })
    await operations.click()
    await database.getByRole('button', { name: 'Rename database', exact: true }).click()
    await expect(database.getByRole('button', { name: 'Rename', exact: true })).toBeDisabled()
    await database.getByLabel('Database name', { exact: true }).fill(renamed)
    await database.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(database.getByRole('heading', { name: original, exact: true })).toBeVisible()
    await database.getByRole('button', { name: 'Rename database', exact: true }).click()
    await database.getByLabel('Database name', { exact: true }).fill(renamed)
    await database.getByRole('button', { name: 'Rename', exact: true }).click()
    await expect(database.getByRole('heading', { name: renamed, exact: true })).toBeVisible()
    await reconnect(renamed)
    await page.goto(dbPath(renamed))
    await expect(database.getByRole('button', { name: table, exact: true })).toBeVisible()
    await runSQL(page, `SELECT id, label FROM ${table};`)
    await expect(page.locator('.resultgrid tbody tr')).toHaveCount(1)
    await expect(page.locator('.resultgrid').getByRole('cell', { name: '7', exact: true })).toBeVisible()
    await expect(page.locator('.resultgrid').getByRole('cell', { name: 'preserved after rename', exact: true })).toBeVisible()

    await reconnect(db)
    await page.goto('/server')
    await expect(server.getByRole('cell', { name: original, exact: true })).toHaveCount(0)
    await expect(server.getByRole('cell', { name: renamed, exact: true })).toBeVisible()
    await server.getByRole('cell', { name: renamed, exact: true }).click()
    await operations.click()
    await database.getByRole('button', { name: 'Drop database', exact: true }).click()
    const confirmName = database.getByLabel(`Type the database name to confirm deletion: ${renamed}`, { exact: true })
    const confirmDrop = database.getByRole('button', { name: 'Confirm drop', exact: true })
    await expect(confirmDrop).toBeDisabled()
    await confirmName.fill(`${renamed}_wrong`)
    await expect(confirmDrop).toBeDisabled()
    await confirmName.fill(renamed)
    await expect(confirmDrop).toBeEnabled()
    await database.getByRole('button', { name: 'Cancel', exact: true }).click()
    await page.goto('/server')
    await expect(server.getByRole('cell', { name: renamed, exact: true })).toBeVisible()
    await server.getByRole('cell', { name: renamed, exact: true }).click()
    await operations.click()
    await database.getByRole('button', { name: 'Drop database', exact: true }).click()
    await confirmName.fill(renamed)
    await confirmDrop.click()
    await expect(page).toHaveURL(/\/server$/)
    await expect(server.getByRole('button', { name: 'Create database', exact: true })).toBeVisible()
    await expect(server.getByRole('cell', { name: renamed, exact: true })).toHaveCount(0)
  } finally {
    await reconnect(db)
    await cleanupDatabases(page, prefix, [original, renamed])
  }
})

test('SQL view creation, source editing, result queries, and cancelled deletion', async ({ page, db, prefix }) => {
  const table = `${prefix}items`
  const view = `${prefix}active`
  await page.goto(dbPath(db))
  try {
    await runSQL(page, `CREATE TABLE ${table} (id INTEGER PRIMARY KEY, label VARCHAR(80), active INTEGER); INSERT INTO ${table} (id, label, active) VALUES (1, 'visible item', 1), (2, 'archived item', 0);`)
    await page.goto(dbPath(db, 'objects/views'))
    const objects = page.locator('.objects-view')
    await expect(objects.getByRole('heading', { name: 'Views', exact: true })).toBeVisible()
    await objects.getByRole('button', { name: 'New object', exact: true }).click()
    await objects.getByLabel('Name', { exact: true }).fill(view)
    await objects.getByLabel('SQL source', { exact: true }).fill(`CREATE VIEW ${view} AS SELECT id, label FROM ${table} WHERE active = 1`)
    await objects.getByRole('button', { name: 'Save SQL source', exact: true }).click()
    await expect(objects.getByRole('status')).toHaveText('Saved.')
    await expect(objects.locator(`[data-object="${view}"]`)).toBeVisible()
    await runSQL(page, `SELECT id, label FROM ${view} ORDER BY id;`)
    await expect(page.locator('.resultgrid tbody tr')).toHaveCount(1)
    await expect(page.locator('.resultgrid').getByRole('cell', { name: 'visible item', exact: true })).toBeVisible()

    await page.goto(dbPath(db, 'objects/views'))
    await objects.getByLabel('Filter objects', { exact: true }).fill(view)
    await objects.locator(`[data-object="${view}"]`).click()
    await expect(objects.getByLabel('Name', { exact: true })).toBeDisabled()
    await expect(objects.getByLabel('SQL source', { exact: true })).toHaveText(new RegExp(table))
    await objects.getByLabel('SQL source', { exact: true }).fill(`CREATE OR REPLACE VIEW ${view} AS SELECT id, label FROM ${table}`)
    await objects.getByRole('button', { name: 'Save SQL source', exact: true }).click()
    await expect(objects.getByRole('status')).toHaveText('Saved.')
    await runSQL(page, `SELECT id, label FROM ${view} ORDER BY id;`)
    await expect(page.locator('.resultgrid tbody tr')).toHaveCount(2)
    await expect(page.locator('.resultgrid').getByRole('cell', { name: 'archived item', exact: true })).toBeVisible()

    await page.goto(dbPath(db, 'objects/views'))
    await objects.locator(`[data-object="${view}"]`).click()
    await objects.getByRole('button', { name: 'Delete', exact: true }).click()
    const confirmation = page.getByRole('alertdialog', { name: 'Delete', exact: true })
    await expect(confirmation).toContainText(view)
    await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(confirmation).toHaveCount(0)
    await objects.getByRole('button', { name: 'Refresh', exact: true }).click()
    await objects.locator(`[data-object="${view}"]`).click()
    await objects.getByRole('button', { name: 'Delete', exact: true }).click()
    await confirmation.getByRole('button', { name: 'Delete', exact: true }).click()
    await expect(objects.getByRole('status')).toHaveText('Deleted.')
    await expect(objects.locator(`[data-object="${view}"]`)).toHaveCount(0)
    await page.reload()
    await expect(objects.getByRole('button', { name: 'New object', exact: true })).toBeEnabled()
    await expect(objects.locator(`[data-object="${view}"]`)).toHaveCount(0)
  } finally {
    await page.goto(dbPath(db))
    await runSQL(page, `DROP VIEW IF EXISTS ${view}; DROP TABLE IF EXISTS ${table};`)
  }
})

test('monitor shows live processes, status and variables, with filtering and refresh', async ({ page }) => {
  const monitorResponses = ['/api/processes', '/api/status', '/api/variables'].map(path =>
    page.waitForResponse(response => new URL(response.url()).pathname === path && response.request().method() === 'GET'),
  )
  await page.getByRole('link', { name: 'Server monitoring', exact: true }).click()
  for (const response of await Promise.all(monitorResponses)) {
    expect(response.ok(), await response.text()).toBeTruthy()
  }
  const monitor = page.locator('.admin-view')
  await expect(monitor.getByRole('heading', { name: 'Server monitoring', exact: true })).toBeVisible()
  await expect(monitor.getByRole('alert')).toHaveCount(0)
  await expect(monitor.locator('table').first().locator('tbody tr').first()).toBeVisible()
  const status = monitor.locator('section').filter({ has: page.getByRole('heading', { name: 'Status', exact: true }) })
  const variables = monitor.locator('section').filter({ has: page.getByRole('heading', { name: 'Variables', exact: true }) })
  await expect(status.locator('tbody tr').first()).toBeVisible()
  await expect(variables.locator('tbody tr').first()).toBeVisible()

  await monitor.getByLabel('Filter metrics', { exact: true }).fill('max_connections')
  const maxConnections = variables.getByRole('row').filter({ has: page.getByRole('cell', { name: 'max_connections', exact: true }) })
  await expect(maxConnections).toBeVisible()
  await expect(maxConnections.getByRole('cell').nth(1)).toHaveText(/^\d+$/)
  await monitor.getByLabel('Filter metrics', { exact: true }).fill('e2e_metric_that_does_not_exist')
  await expect(status.locator('tbody tr')).toHaveCount(0)
  await expect(variables.locator('tbody tr')).toHaveCount(0)
  await monitor.getByLabel('Filter metrics', { exact: true }).fill('')
  await expect(status.locator('tbody tr').first()).toBeVisible()

  const refreshed = ['/api/processes', '/api/status', '/api/variables'].map(path =>
    page.waitForResponse(response => new URL(response.url()).pathname === path && response.request().method() === 'GET'),
  )
  await monitor.getByRole('button', { name: 'Refresh', exact: true }).click()
  for (const response of await Promise.all(refreshed)) {
    expect(response.ok(), await response.text()).toBeTruthy()
  }
  await expect(monitor.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled()
  await expect(monitor.getByRole('alert')).toHaveCount(0)
  await expect(monitor.locator('table').first().locator('tbody tr').first()).toBeVisible()
})

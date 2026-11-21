import { test, expect, runSQL, tablePath, dbPath } from './fixtures'

const navigationKey = 'db-manager:navigation:v1'

test('direct operations load resolves a view with the sidebar collapsed', async ({ page, db, engine, prefix }) => {
 const table = `${prefix}_base`, view = `${prefix}_view`
 try {
  await runSQL(page, `CREATE TABLE ${table}(id INTEGER PRIMARY KEY); CREATE VIEW ${view} AS SELECT id FROM ${table};`)
  await page.setViewportSize({ width: 680, height: 900 })
  await page.goto(tablePath(db, view, 'operations'))
  await expect(page.locator('.app-sidebar')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Truncate', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Drop', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Drop', exact: true }).click()
  await expect(page.locator('.ui-popconfirm')).toContainText(view)
  const deletion = page.waitForResponse(response => response.request().method() === 'DELETE' && new URL(response.url()).pathname.endsWith(`/tables/${view}`))
  await page.locator('.ui-popconfirm').getByRole('button', { name: /^(Confirm|OK)$/ }).click()
  const response = await deletion
  expect(new URL(response.url()).searchParams.get('isView')).toBe('true')
  expect(response.ok(), `DELETE returned HTTP ${response.status()}`).toBe(true)
  await expect(page).toHaveURL(`${dbPath(db)}${engine === 'postgres' ? '?schema=public' : ''}`)
  const catalog = await page.request.get(`/api/databases/${encodeURIComponent(db)}/tables`)
  expect(catalog.ok()).toBe(true)
  expect((await catalog.json()).tables).not.toEqual(expect.arrayContaining([expect.objectContaining({ name: view })]))
 } finally {
  await runSQL(page, `DROP VIEW IF EXISTS ${view}; DROP TABLE IF EXISTS ${table};`)
 }
})

test('a selected duplicate keyless row cannot start a bulk update', async ({ page, db, prefix }) => {
 const table = `${prefix}_keyless`
 try {
  await runSQL(page, `CREATE TABLE ${table}(title VARCHAR(100)); INSERT INTO ${table} VALUES ('same'),('same');`)
  await page.goto(tablePath(db, table))
  const rows = page.locator('.datagrid tbody tr')
  await expect(rows).toHaveCount(2)
  await rows.nth(0).getByRole('checkbox').check()
  await expect(page.getByRole('button', { name: /^Edit selected/ })).toBeDisabled()
  await expect(page.locator('[data-bulk-edit-unavailable]')).toContainText('primary key')
  await page.reload()
  await expect(rows).toHaveCount(2)
  for (const row of await rows.all()) await expect(row).toContainText('same')
 } finally { await runSQL(page, `DROP TABLE IF EXISTS ${table}`) }
})

test('MySQL and MariaDB TIME durations remain visible and editable through row forms', async ({ page, db, engine, prefix }) => {
 test.skip(engine === 'postgres', 'MySQL/MariaDB TIME stores signed durations')
 const table = `${prefix}_duration`
 try {
  await runSQL(page, `CREATE TABLE ${table}(id INTEGER PRIMARY KEY, elapsed TIME(6) NOT NULL, note VARCHAR(100) NOT NULL DEFAULT ''); INSERT INTO ${table} VALUES(1,'36:00:00.123456',''),(2,'-01:30:00.000001','');`)
  await page.goto(tablePath(db, table))
  const rows = page.locator('.datagrid tbody tr')
  const row = (id: number) => rows.filter({ has: page.getByRole('cell', { name: String(id), exact: true }) })
  const modal = page.locator('.row-modal')
  const field = (name: string) => modal.locator('.row-modal__field').filter({ has: page.locator('.row-modal__label').getByText(name, { exact: true }) })
  for (const [id, value] of [[1, '36:00:00.123456'], [2, '-01:30:00.000001']] as const) {
   await row(id).getByRole('button', { name: 'Edit', exact: true }).click()
   await expect(field('elapsed').locator('input')).toHaveAttribute('type', 'text')
   await expect(field('elapsed').locator('input')).toHaveValue(value)
   await modal.getByRole('button', { name: 'Cancel', exact: true }).click()
   await expect(modal).toBeHidden()
  }
  await row(1).getByRole('button', { name: 'Edit', exact: true }).click()
  await field('note').locator('input').fill('unrelated edit')
  const patch = page.waitForRequest(request => request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith(`/tables/${table}/rows`))
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  expect((await patch).postDataJSON().values).toEqual({ note: 'unrelated edit' })
  await expect(modal).toBeHidden()
  await expect(row(1)).toContainText('unrelated edit')
  await row(1).getByRole('button', { name: 'Edit', exact: true }).click()
  await field('elapsed').locator('input').fill('-36:10:11.123456')
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(modal).toBeHidden()
  await expect(row(1)).toContainText('-36:10:11.123456')
  await page.getByRole('button', { name: 'Add row', exact: true }).click()
  await field('id').locator('input').fill('3')
  await field('elapsed').locator('input').fill('48:01:02.123456')
  await modal.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(modal).toBeHidden()
  await expect(row(3)).toContainText('48:01:02.123456')
 } finally { await runSQL(page, `DROP TABLE IF EXISTS ${table}`) }
})

test('search URL context survives a competing sidebar catalog load and later manual selection', async ({ page, db, prefix }) => {
 const table = `${prefix}_target`, manual = `${prefix}_manual`
 const catalogPath = `/api/databases/${encodeURIComponent(db)}/tables`
 let releaseDatabases!: () => void, releaseSecond!: () => void, secondStarted!: () => void, firstFinished!: () => void
 const databasesGate = new Promise<void>(resolve => { releaseDatabases = resolve })
 const secondGate = new Promise<void>(resolve => { releaseSecond = resolve })
 const secondSeen = new Promise<void>(resolve => { secondStarted = resolve })
 const firstDone = new Promise<void>(resolve => { firstFinished = resolve })
 let catalogRequests = 0
 try {
  await runSQL(page, `CREATE TABLE ${table}(id INTEGER PRIMARY KEY,title VARCHAR(100)); CREATE TABLE ${manual}(id INTEGER PRIMARY KEY,title VARCHAR(100));`)
  await page.addInitScript(key => localStorage.setItem(key, JSON.stringify({ collapsed: false, width: 220, recent: [], favorites: [] })), navigationKey)
  await page.route(url => url.pathname === '/api/databases', async route => {
   const response = await route.fetch()
   await databasesGate
   await route.fulfill({ response })
  })
  await page.route(url => url.pathname === catalogPath, async route => {
   const number = ++catalogRequests
   const response = await route.fetch()
   if (number === 1) {
    releaseDatabases()
    await secondSeen
    await route.fulfill({ response })
    firstFinished()
   } else if (number === 2) {
    secondStarted()
    await secondGate
    await route.fulfill({ response })
   } else await route.fulfill({ response })
  })
  await page.goto(`${dbPath(db, 'search')}?table=${encodeURIComponent(table)}`)
  await firstDone
  releaseSecond()
  const target = page.getByRole('checkbox', { name: table, exact: true })
  const choice = page.getByRole('checkbox', { name: manual, exact: true })
  await expect(target).toBeChecked()
  await target.uncheck(); await choice.check()
  await page.getByRole('button', { name: 'Collapse sidebar', exact: true }).click()
  await page.getByRole('button', { name: 'Expand sidebar', exact: true }).click()
  await expect.poll(() => catalogRequests).toBeGreaterThanOrEqual(3)
  await expect(page.locator('.search-view fieldset')).toBeEnabled()
  await expect(target).not.toBeChecked()
  await expect(choice).toBeChecked()
 } finally {
  releaseDatabases(); releaseSecond(); secondStarted(); firstFinished()
  await page.unrouteAll({ behavior: 'wait' })
  await runSQL(page, `DROP TABLE IF EXISTS ${table}; DROP TABLE IF EXISTS ${manual};`)
 }
})

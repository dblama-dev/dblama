import { test, expect, runSQL, selectOption, tablePath, dbPath } from './fixtures'
import type { Page } from '@playwright/test'

async function addColumn(page: Page, name: string, type: string) {
  await page.getByTestId('struct-add').click()
  const row = page.getByTestId('struct-row').nth(await page.getByTestId('struct-row').count() - 1)
  await row.locator('.struct-field--name input').fill(name)
  await selectOption(page, row.locator('.struct-field--type .n-select'), type)
  return row
}

test('autoincrement can be turned off by a second click and moved to another column', async ({ page, db }) => {
  await page.goto(tablePath(db, '_new', 'structure'))
  const first = await addColumn(page, 'id', 'int')
  const second = await addColumn(page, 'other_id', 'int')
  const toggle = first.locator('.struct-field--autoinc input[type=radio], .struct-field--autoinc [role=checkbox]')
  const button = first.locator('.struct-field--autoinc .n-radio, .struct-field--autoinc .n-checkbox')
  await button.click()
  await expect(toggle).toBeChecked()
  await button.click()
  await expect(toggle).not.toBeChecked()
  await button.click()
  const other = second.locator('.struct-field--autoinc input[type=radio], .struct-field--autoinc [role=checkbox]')
  await second.locator('.struct-field--autoinc .n-radio, .struct-field--autoinc .n-checkbox').click()
  await expect(other).toBeChecked()
  await expect(toggle).not.toBeChecked()
})

test('create generated primary key, insert without ID, edit structure and reload', async ({ page, db, prefix }) => {
  const table = `${prefix}_items`
  const renamed = `${prefix}_renamed`
  try {
    await page.goto(dbPath(db))
    await page.locator('.db-view').getByRole('button', { name: 'New table', exact: true }).click()
    await page.locator('.struct-field--table-name input').fill(table)
    const id = await addColumn(page, 'id', 'int')
    await id.locator('.struct-field--nullable [role=checkbox]').click()
    await id.locator('.struct-field--autoinc .n-radio, .struct-field--autoinc .n-checkbox').click()
    await addColumn(page, 'title', 'text')
    await page.getByRole('button', { name: 'Add index', exact: true }).click()
    const index = page.getByTestId('index-row')
    await selectOption(page, index.locator('.struct-field--indextype .n-select'), 'PRIMARY')
    await selectOption(page, index.locator('.struct-field--indexcolumns .n-select'), 'id')
    await page.getByTestId('struct-save').click()
    await expect(page).toHaveURL(new RegExp(`/table/${table}\\?view=structure`))
    await page.reload()
    await expect(page.getByTestId('struct-row').first().locator('.struct-field--autoinc input[type=radio], .struct-field--autoinc [role=checkbox]')).toBeChecked()
    await expect(page.getByTestId('index-row')).toContainText('PRIMARY')
    await runSQL(page, `INSERT INTO ${table} (title) VALUES ('first'), ('second'); SELECT id, title FROM ${table} ORDER BY id;`)
    await expect(page.locator('.resultgrid').last()).toContainText('first')
    await expect(page.locator('.resultgrid').last()).toContainText('second')
    await page.goto(tablePath(db, table))
    const dataRows = page.locator('.datagrid tbody tr')
    await expect(dataRows).toHaveCount(2)
    await expect(dataRows.first().locator('td').nth(1)).toHaveText('1')
    await expect(dataRows.last().locator('td').nth(1)).toHaveText('2')
    await page.goto(tablePath(db, table, 'structure'))
    await addColumn(page, 'note', 'text')
    await page.locator('.struct-field--table-name input').fill(renamed)
    await page.getByTestId('struct-save').click()
    await expect(page).toHaveURL(new RegExp(`/table/${renamed}\\?view=structure`))
    await page.reload()
    await expect(page.getByTestId('struct-row')).toHaveCount(3)
    await expect(page.getByTestId('struct-row').last().locator('.struct-field--name input')).toHaveValue('note')
    await page.goto(tablePath(db, renamed))
    await expect(page.locator('.datagrid tbody tr')).toHaveCount(2)
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${renamed}; DROP TABLE IF EXISTS ${table};`)
  }
})

test('existing table can disable autoincrement and retain its primary key', async ({ page, db, engine, prefix }) => {
  const table = `${prefix}_toggle`
  try {
    await runSQL(page, `CREATE TABLE ${table} (id ${engine === 'postgres' ? 'SERIAL' : 'INT AUTO_INCREMENT'} PRIMARY KEY, title TEXT);`)
    await page.goto(tablePath(db, table, 'structure'))
    const toggle = page.getByTestId('struct-row').first().locator('.struct-field--autoinc input[type=radio], .struct-field--autoinc [role=checkbox]')
    const button = page.getByTestId('struct-row').first().locator('.struct-field--autoinc .n-radio, .struct-field--autoinc .n-checkbox')
    await button.click()
    const saved = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/tables/${table}`))
    await page.getByTestId('struct-save').click()
    expect((await saved).ok()).toBe(true)
    await page.reload()
    await expect(toggle).not.toBeChecked()
    await expect(page.getByTestId('index-row')).toContainText('PRIMARY')
  } finally { await runSQL(page, `DROP TABLE IF EXISTS ${table};`) }
})

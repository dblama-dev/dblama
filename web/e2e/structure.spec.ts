import { test, expect, runSQL, selectOption, tablePath, dbPath } from './fixtures'
import type { Page } from '@playwright/test'

async function addColumn(page: Page, name: string, type: string) {
  await page.getByTestId('struct-add').click()
  const row = page.getByTestId('struct-row').nth(await page.getByTestId('struct-row').count() - 1)
  await row.locator('.struct-field--name input').fill(name)
  await selectOption(page, row.locator('.struct-field--type .ui-select'), type)
  return row
}

test('column defaults and ordered enum values survive a structure edit', async ({ page, db, prefix, engine }) => {
  const table = `${prefix}_defaults`
  const choices = engine === 'postgres' ? '' : ", choice ENUM('', 'red, green', ' later') DEFAULT 'red, green'"
  try {
    await runSQL(page, `CREATE TABLE ${table} (id INT PRIMARY KEY, blank VARCHAR(30) NOT NULL DEFAULT '', keyword VARCHAR(30) NOT NULL DEFAULT 'NULL', note VARCHAR(30)${choices});`)
    await page.goto(tablePath(db, table, 'structure'))
    const rows = page.getByTestId('struct-row')
    await expect(rows.nth(1).locator('summary')).toContainText("DEFAULT ''")
    await expect(rows.nth(2).locator('summary')).toContainText("DEFAULT 'NULL'")
    const note = rows.nth(3)
    await note.locator('summary').click()
    await note.getByLabel('Default', { exact: true }).selectOption('literal')
    if (engine !== 'postgres') {
      const choice = rows.nth(4)
      await choice.locator('summary').click()
      await expect(choice.getByLabel('Value 1', { exact: true })).toHaveValue('')
      await expect(choice.getByLabel('Value 2', { exact: true })).toHaveValue('red, green')
      await expect(choice.getByLabel('Value 3', { exact: true })).toHaveValue(' later')
      await choice.getByRole('button', { name: 'Add value' }).click()
      await choice.getByLabel('Value 4', { exact: true }).fill('new, choice')
      await page.getByRole('button', { name: 'Add column after keyword', exact: true }).click()
      const added = rows.nth(3)
      await expect(added.locator('.struct-field--name input')).toBeFocused()
      await added.locator('.struct-field--name input').fill('middle_col')
      await selectOption(page, added.locator('.struct-field--type .ui-select'), 'int')
    } else {
      await expect(page.getByRole('button', { name: /Add column after/ })).toHaveCount(0)
    }
    const saved = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/tables/${table}`))
    await page.getByTestId('struct-save').click()
    const response = await saved
    expect(response.ok(), response.ok() ? undefined : await response.text()).toBe(true)
    await page.reload()
    if (engine !== 'postgres') await expect(rows.nth(3).locator('summary')).toContainText('middle_col')
    await expect(rows.nth(engine === 'postgres' ? 3 : 4).locator('summary')).toContainText("DEFAULT ''")
    const enumCondition = engine === 'postgres' ? '' : " AND choice = 'red, green'"
    await runSQL(page, `INSERT INTO ${table} (id) VALUES (1); SELECT 'defaults applied' AS result FROM ${table} WHERE blank = '' AND keyword = 'NULL' AND note = ''${enumCondition};`)
    await expect(page.locator('.resultgrid').last()).toContainText('defaults applied')
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${table};`)
  }
})

test('column checkboxes stay together and wrap inside a narrow editor', async ({ page, db, engine }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto(tablePath(db, '_new', 'structure'))
  const column = await addColumn(page, 'quantity', 'int')
  const labels = engine === 'postgres' ? ['Nullable', 'Auto-number'] : ['Nullable', 'unsigned', 'AUTO_INCREMENT']
  const controls = labels.map(name =>
    column.getByRole('checkbox', { name, exact: true }),
  )
  for (const control of controls) await expect(control).toBeVisible()
  const desktop = await Promise.all(controls.map(control => control.boundingBox()))
  for (const box of desktop) expect(box!.y).toBeCloseTo(desktop[0]!.y, 0)

  await page.setViewportSize({ width: 320, height: 800 })
  const narrow = await Promise.all(controls.map(control => control.boundingBox()))
  const editor = (await column.boundingBox())!
  for (const box of narrow) {
    expect(box!.x).toBeGreaterThanOrEqual(editor.x)
    expect(box!.x + box!.width).toBeLessThanOrEqual(editor.x + editor.width)
  }
  if (engine !== 'postgres') expect(narrow[2]!.y).toBeGreaterThan(narrow[0]!.y)
})

test('autoincrement can be turned off by a second click and moved to another column', async ({ page, db, engine }) => {
  await page.goto(tablePath(db, '_new', 'structure'))
  const first = await addColumn(page, 'id', 'int')
  const second = await addColumn(page, 'other_id', 'int')
  const name = engine === 'postgres' ? 'Auto-number' : 'AUTO_INCREMENT'
  const toggle = first.getByRole('checkbox', { name, exact: true })
  const caption = first.getByText(name, { exact: true })
  await caption.click()
  await expect(toggle).toBeChecked()
  await caption.click()
  await expect(toggle).not.toBeChecked()
  await toggle.press('Space')
  const other = second.getByRole('checkbox', { name, exact: true })
  await other.click()
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
    await id.locator('.struct-field--nullable input[type=checkbox]').click()
    await id.locator('.struct-field--autoinc input[type=checkbox]').click()
    await addColumn(page, 'title', 'text')
    await page.getByRole('button', { name:'Indexes', exact:true }).click()
    await page.getByRole('button', { name: 'Add index', exact: true }).click()
    const index = page.getByTestId('index-row')
    await selectOption(page, index.locator('.struct-field--indextype .ui-select'), 'PRIMARY')
    await selectOption(page, index.locator('.struct-field--indexcolumns .ui-select'), 'id')
    await page.getByTestId('struct-save').click()
    await expect(page).toHaveURL(new RegExp(`/table/${table}\\?view=structure`))
    await page.reload()
    await expect(page.getByTestId('struct-row').first().locator('.struct-field--autoinc input[type=checkbox]')).toBeChecked()
    await page.getByRole('button', { name:'Indexes', exact:true }).click()
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
    const toggle = page.getByTestId('struct-row').first().locator('.struct-field--autoinc input[type=checkbox]')
    const button = page.getByTestId('struct-row').first().locator('.struct-field--autoinc input[type=checkbox]')
    await page.getByTestId('struct-row').first().locator('summary').click()
    await button.click()
    const saved = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/tables/${table}`))
    await page.getByTestId('struct-save').click()
    expect((await saved).ok()).toBe(true)
    await page.reload()
    await expect(toggle).not.toBeChecked()
    await page.getByRole('button', { name:'Indexes', exact:true }).click()
    await expect(page.getByTestId('index-row')).toContainText('PRIMARY')
  } finally { await runSQL(page, `DROP TABLE IF EXISTS ${table};`) }
})

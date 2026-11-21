import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import type { APIRequestContext, Page } from '@playwright/test'
import { dbPath, expect, loginRequest, runSQL, selectOption, test } from './fixtures'

const headers = { 'X-Requested-With': 'XMLHttpRequest' }

interface Scope {
  database: string
  schema: string
  marker: string
  sequenceStart: number
}

async function fixtureSQL(request: APIRequestContext, database: string, sql: string): Promise<void> {
  const response = await request.post(`/api/databases/${encodeURIComponent(database)}/query`, {
    headers,
    data: { sql, queryID: randomUUID() },
  })
  await expect(response).toBeOK()
}

async function chooseDatabase(page: Page, database: string): Promise<void> {
  const picker = page.getByTestId('database-select')
  await selectOption(page, picker, database)
  await expect(page).toHaveURL(url => url.pathname === dbPath(database))
  await expect(picker.locator('input')).toHaveValue(database)
}

test.use({ loginDatabase: '' })
test.skip(({ engine }) => engine !== 'postgres', 'Database scoping regression for PostgreSQL')

test('blank PostgreSQL login lets each tab browse, query, and export its selected database', async ({
  page, context, request, playwright, baseURL, db, prefix,
}) => {
  const base: Scope = { database: db, schema: `${prefix}_base_schema`, marker: 'base database row', sequenceStart: 41 }
  const other: Scope = { database: `${prefix}_other`, schema: `${prefix}_other_schema`, marker: 'other database row', sequenceStart: 73 }
  const table = `${prefix}_items`
  const sequence = `${prefix}_sequence`

  async function seed(setup: APIRequestContext, scope: Scope): Promise<void> {
    await fixtureSQL(setup, scope.database, `
      CREATE TABLE ${scope.schema}.${table} (label TEXT NOT NULL);
      INSERT INTO ${scope.schema}.${table} VALUES ('${scope.marker}');
      CREATE SEQUENCE ${scope.schema}.${sequence} START WITH ${scope.sequenceStart};
    `)
  }

  async function browse(tab: Page, selected: Scope, unselected: Scope): Promise<void> {
    const schemaPicker = tab.getByTestId('schema-select')
    // Wait for the selected database's schemas before checking that the other database's schema is absent.
    await schemaPicker.click()
    const options = tab.locator('.ui-select__option:visible')
    const schema = options.getByText(selected.schema, { exact: true })
    await expect(schema).toBeVisible()
    await expect(options.getByText(unselected.schema, { exact: true })).toHaveCount(0)
    await schema.click()
    await expect(schemaPicker.locator('input')).toHaveValue(selected.schema)
    await expect(tab.getByRole('tree').getByText(sequence, { exact: true })).toBeVisible()
    await tab.locator('.db-view').getByRole('button', { name: table, exact: true }).click()
    await expect(tab.locator('.datagrid').getByRole('cell', { name: selected.marker, exact: true })).toBeVisible()
  }

  async function verifyQuery(tab: Page, scope: Scope): Promise<void> {
    await runSQL(tab, `SELECT current_database(), label, last_value
      FROM ${scope.schema}.${table} CROSS JOIN ${scope.schema}.${sequence}`)
    await expect(tab.locator('.resultgrid tbody td')).toHaveText([
      scope.database, scope.marker, String(scope.sequenceStart),
    ])
  }

  // Provision through explicitly scoped setup sessions so the browser starts with a blank login.
  await expect(await request.post('/api/session', { headers, data: loginRequest(base.database) })).toBeOK()
  let baseSchemaCreated = false
  let otherDatabaseCreated = false
  try {
    await expect(await request.post('/api/databases', { headers, data: { name: other.database } })).toBeOK()
    otherDatabaseCreated = true
    await fixtureSQL(request, base.database, `CREATE SCHEMA ${base.schema}`)
    baseSchemaCreated = true
    await seed(request, base)

    const setup = await playwright.request.newContext({ baseURL })
    try {
      await expect(await setup.post('/api/session', { headers, data: loginRequest(other.database) })).toBeOK()
      await fixtureSQL(setup, other.database, `CREATE SCHEMA ${other.schema}`)
      await seed(setup, other)
    } finally {
      await setup.delete('/api/session', { headers })
      await setup.dispose()
    }

    // Refresh the sidebar's database list after provisioning the second database.
    await page.reload()
    await expect(page.getByTestId('database-select').locator('input')).toHaveValue('')
    await expect(page.getByTestId('schema-select')).toHaveCount(0)
    await expect(page.getByRole('tree')).toHaveCount(0)
    await chooseDatabase(page, other.database)
    await runSQL(page, 'SELECT current_database()')
    await expect(page.locator('.resultgrid tbody td')).toHaveText([other.database])

    const secondTab = await context.newPage()
    await secondTab.goto('/server')
    await expect(secondTab.getByTestId('database-select').locator('input')).toHaveValue('')
    await chooseDatabase(secondTab, base.database)
    await chooseDatabase(page, other.database)
    await browse(page, other, base)
    await browse(secondTab, base, other)
    await verifyQuery(page, other)
    await verifyQuery(secondTab, base)

    await chooseDatabase(page, base.database)
    await chooseDatabase(secondTab, other.database)
    await verifyQuery(page, base)
    await browse(secondTab, other, base)
    await secondTab.getByRole('navigation', { name: 'Database tools' }).getByRole('link', { name: 'Export', exact: true }).click()
    await expect(secondTab).toHaveURL(`${dbPath(other.database, 'export')}?schema=${other.schema}`)
    await secondTab.getByRole('checkbox', { name: table, exact: true }).check()
    const download = secondTab.waitForEvent('download')
    await secondTab.getByTestId('export-submit').click()
    const sql = await readFile(await (await download).path(), 'utf8')
    expect(sql).toContain(other.marker)
    expect(sql).not.toContain(base.marker)
  } finally {
    try {
      // Disconnect the workspace before dropping the database it opened.
      await page.request.delete('/api/session', { headers })
      if (baseSchemaCreated) await fixtureSQL(request, base.database, `DROP SCHEMA ${base.schema} CASCADE`)
    } finally {
      try {
        if (otherDatabaseCreated) {
          await expect(await request.delete(`/api/databases/${encodeURIComponent(other.database)}`, { headers })).toBeOK()
        }
      } finally {
        await request.delete('/api/session', { headers })
      }
    }
  }
})

import { dbPath, expect, runSQL, test } from './fixtures'

test('mobile catalog keeps table names readable and database actions in scope', async ({ page, db, engine, prefix }) => {
  const table = prefix + '_orders'
  await runSQL(page, `CREATE TABLE ${table} (id INTEGER PRIMARY KEY)`)
  try {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(dbPath(db))
    await page.getByRole('searchbox', { name: 'Filter tables' }).fill(table)
    const tableName = page.locator('.catalog-grid').getByRole('button', { name: table, exact: true })
    await expect(tableName).toBeVisible()
    const bounds = await tableName.boundingBox()
    expect(bounds!.width).toBeGreaterThan(130)
    expect(bounds!.height).toBeLessThan(48)
    const checkbox = page.getByRole('checkbox', { name: table, exact: true })
    await checkbox.check()
    const bulk = page.locator('.db-view__bulk')
    await expect(bulk.getByRole('button', { name: 'Drop', exact: true })).toBeVisible()
    const selectedBar = await bulk.boundingBox()
    const firstRow = await tableName.boundingBox()
    expect(selectedBar!.y + selectedBar!.height).toBeLessThanOrEqual(firstRow!.y)
    await checkbox.uncheck()
    await expect(bulk).toHaveCount(0)
    await page.getByRole('button', { name: 'Expand sidebar' }).click()
    await page.locator('.advanced-navigation summary').click()
    await page.getByRole('link', { name: 'Database actions', exact: true }).click()
    await expect(page).toHaveURL(dbPath(db, 'operations') + (engine === 'postgres' ? '?schema=public' : ''))
    await expect(page.getByRole('button', { name: 'Rename database', exact: true })).toBeVisible()
    await page.goto(dbPath(db))
    await page.locator('.db-view').getByRole('link', { name: 'Relationships', exact: true }).click()
    await expect(page).toHaveURL(dbPath(db, 'relationships') + (engine === 'postgres' ? '?schema=public' : ''))
    await page.getByRole('button', { name: 'Settings', exact: true }).click()
    await page.getByRole('dialog').getByRole('radio', { name: 'Dark' }).check()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await expect(page.locator('html')).toHaveCSS('opacity', '1')
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${table}`)
  }
})

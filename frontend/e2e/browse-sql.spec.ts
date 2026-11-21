import { expect, runSQL, tablePath, test } from './fixtures'

test('table browser shows the executed SELECT and its bound values', async ({ page, db, prefix, engine }) => {
  await runSQL(page, `CREATE TABLE ${prefix} (id INT PRIMARY KEY, label VARCHAR(80));
    INSERT INTO ${prefix} VALUES (1, 'O''Brien')`)
  try {
    const filters = [{ column: 'label', op: '=', value: "O'Brien" }]
    const query = new URLSearchParams({ filters: JSON.stringify(filters), sort: JSON.stringify({ column: 'id', desc: true }) })
    await page.goto(tablePath(db, prefix) + '&' + query)
    const grid = page.locator('.datagrid')
    await expect(grid.getByRole('cell', { name: "O'Brien", exact: true })).toBeVisible()
    await grid.getByText('Browse SQL', { exact: true }).click()
    const statement = grid.locator('details[open] pre')
    const quote = engine === 'postgres' ? '"' : '`'
    const scope = engine === 'postgres' ? 'public' : db
    const bind = engine === 'postgres' ? ['$1', '$2', '$3'] : ['?', '?', '?']
    await expect(statement).toHaveText(`SELECT * FROM ${quote}${scope}${quote}.${quote}${prefix}${quote} WHERE ${quote}label${quote} = ${bind[0]} ORDER BY ${quote}id${quote} DESC LIMIT ${bind[1]} OFFSET ${bind[2]}`)
    await expect(grid.locator('details[open] output')).toHaveText(JSON.stringify(["O'Brien", 50, 0]))
    await page.setViewportSize({ width: 390, height: 844 })
    await grid.getByRole('button', { name: 'Filters (1)', exact: true }).click()
    const filter = grid.locator('.datagrid__filter-value input')
    await expect(filter).toHaveValue("O'Brien")
    const bounds = await filter.boundingBox()
    expect(bounds).not.toBeNull()
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${prefix}`)
  }
})

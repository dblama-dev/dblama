import { dbPath, expect, runSQL, test } from './fixtures'

test('sidebar database search, scoped tools, resizing and mobile navigation', async ({ page, db, engine }) => {
  const scope = engine === 'postgres' ? '?schema=public' : ''
  await page.goto(dbPath(db))
  const sidebar = page.locator('.app-sidebar')
  const picker = sidebar.getByTestId('database-select')
  await picker.click()
  await picker.locator('input').fill('no-matching-database-123')
  await expect(page.locator('.ui-select__option:visible')).toHaveCount(0)
  await picker.locator('input').fill(db)
  await page.locator('.ui-select__option:visible').getByText(db, { exact: true }).click()
  const tools = sidebar.getByRole('navigation', { name: 'Database tools' })
  const sql = tools.getByRole('link', { name: 'SQL query', exact: true })
  await expect(sql).toHaveAttribute('href', dbPath(db, 'console') + scope)
  await expect(tools.getByRole('link')).toHaveCount(4)
  const more = sidebar.locator('details.advanced-navigation')
  await expect(more.getByRole('link')).toHaveCount(0)
  await more.locator('summary').click()
  await expect(more.getByRole('link', { name: 'Relationships' })).toHaveAttribute('href', dbPath(db, 'relationships') + scope)

  const handle = page.getByRole('separator', { name: 'Sidebar width' })
  await handle.focus()
  await handle.press('End')
  await expect(handle).toHaveAttribute('aria-valuenow', '420')
  const bounds = await handle.boundingBox()
  const x = bounds!.x + 2, y = bounds!.y + 20
  await page.mouse.move(x, y)
  await page.mouse.down()
  await page.mouse.move(x - 40, y)
  await page.mouse.up()
  await expect(handle).toHaveAttribute('aria-valuenow', '380')

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('.breadcrumbs [aria-current="page"]:visible')).toHaveText(db)
  await page.getByRole('button', { name: 'Expand sidebar' }).click()
  await expect(sidebar).toBeVisible()
  const box = await sidebar.boundingBox()
  expect(box!.x + box!.width).toBeLessThanOrEqual(354)
  await sql.click()
  await expect(page).toHaveURL(dbPath(db, 'console') + scope)
  await expect(sidebar).toHaveCount(0)
  await expect(page.locator('.cm-content')).toBeVisible()
  await page.getByRole('button', { name: 'Theme', exact: true }).click()
  await page.getByRole('button', { name: 'RU', exact: true }).click()
  await page.getByRole('button', { name: 'Развернуть панель' }).click()
  await expect(sidebar.getByRole('link', { name: 'SQL-запрос', exact: true })).toBeVisible()
  await expect(sidebar.getByText('Другие инструменты', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(sidebar).toHaveCount(0)
})


test('quick search opens a scoped table with the sidebar collapsed', async ({ page, db, prefix, engine }) => {
  const table = prefix + '_lookup'
  const scope = engine === 'postgres' ? '?schema=public' : ''
  await runSQL(page, `CREATE TABLE ${table} (id INTEGER PRIMARY KEY)`)
  try {
    await page.getByRole('button', { name: 'Collapse sidebar' }).click()
    // A reload empties the Pinia catalog. Search must work independently of the tree.
    await page.reload()
    await expect(page.locator('.app-sidebar')).toHaveCount(0)
    await page.getByRole('button', { name: 'Quick search' }).waitFor()
    await page.keyboard.press('Control+k')
    const dialog = page.locator('.jump-card')
    const input = dialog.getByRole('searchbox', { name: 'Quick search' })
    await input.fill(table)
    await expect(dialog.getByRole('button', { name: table + ' Tables' })).toBeVisible()
    await input.press('Enter')
    await expect(page).toHaveURL(dbPath(db, 'table/' + table) + scope)
    await expect(page.locator('.breadcrumbs [aria-current="page"]')).toHaveText(table)
  } finally {
    await runSQL(page, `DROP TABLE ${table}`)
  }
})

import { dbPath, expect, runSQL, test } from './fixtures'

test('open Insert from the database, save consecutive rows and recover from a duplicate', async ({ page, db, prefix }) => {
  const table = prefix + '_entries'
  await runSQL(page, `CREATE TABLE ${table} (id INT PRIMARY KEY, note VARCHAR(80) NOT NULL DEFAULT 'server value')`)
  try {
    await page.goto(dbPath(db))
    const tableRow = page.getByRole('row').filter({ has: page.getByRole('button', { name: table, exact: true }) })
    await tableRow.getByRole('button', { name: 'Insert', exact: true }).click()
    const form = page.locator('.row-modal:visible')
    const id = form.locator('.row-modal__field').filter({ has: page.getByText('id', { exact: true }) }).locator('input').first()
    await id.fill('7')
    await form.getByRole('button', { name: 'Save and add another', exact: true }).click()
    await expect(id).toHaveValue('')
    await id.fill('7')
    await form.getByRole('button', { name: 'Save and add another', exact: true }).click()
    await expect(form.getByRole('alert')).toBeVisible()
    await expect(id).toHaveValue('7')
    await id.fill('8')
    await form.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(form).toHaveCount(0)
    await page.reload()
    await expect(page.locator('.row-modal:visible')).toHaveCount(0)
    await expect(page.locator('.datagrid').getByText('server value', { exact: true })).toHaveCount(2)
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${table}`)
  }
})

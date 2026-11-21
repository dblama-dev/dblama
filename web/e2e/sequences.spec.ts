import { dbPath, expect, runSQL, test } from './fixtures'

test('sequence restart and increment are applied by PostgreSQL', async ({ page, db, engine, prefix }) => {
  test.skip(engine !== 'postgres', 'Sequence editing requires PostgreSQL')
  const name = `${prefix}_sequence`
  const node = page.locator('.ui-tree-node').filter({ has: page.getByText(name, { exact: true }) })
  const form = page.locator('.sequence-form')
  await page.goto(dbPath(db))
  try {
    await page.getByTestId('tree-add-sequence').click()
    await page.getByTestId('sequence-name').locator('input').fill(name)
    await page.getByTestId('sequence-create').click()
    await node.getByTitle('Alter sequence').click()
    await form.locator('label').filter({ hasText: 'Increment (nonzero)' }).locator('input').fill('5')
    await form.locator('label').filter({ hasText: 'Restart with (optional)' }).locator('input').fill('42')
    await page.getByTestId('sequence-create').click()
    await expect(form).toHaveCount(0)

    await runSQL(page, `SELECT nextval('public.${name}') AS first_value; SELECT nextval('public.${name}') AS second_value;`)
    await expect(page.locator('.resultgrid tbody td')).toHaveText(['42', '47'])
  } finally {
    await runSQL(page, `DROP SEQUENCE IF EXISTS public.${name};`)
  }
})

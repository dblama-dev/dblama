import { dbPath, expect, runSQL, test } from './fixtures'

test('browse composite foreign keys and open the referenced table structure', async ({ page, db, prefix }) => {
  const parent = prefix + '_parent'
  const child = prefix + '_child'
  const constraint = prefix + '_fk'
  await runSQL(page, `CREATE TABLE ${parent} (tenant INT, id INT, PRIMARY KEY (tenant, id));
    CREATE TABLE ${child} (org INT, item INT, CONSTRAINT ${constraint}
    FOREIGN KEY (org, item) REFERENCES ${parent} (tenant, id) ON DELETE CASCADE);`)
  try {
    await page.goto(dbPath(db))
    await page.getByRole('link', { name: 'Relationships', exact: true }).click()
    await page.getByRole('searchbox', { name: 'Filter relationships' }).fill(parent)
    const relation = page.getByRole('article').filter({ hasText: constraint })
    await expect(relation).toContainText('CASCADE')
    await expect(relation.getByText('org', { exact: true })).toBeVisible()
    await expect(relation.getByText('tenant', { exact: true })).toBeVisible()
    await expect(relation.getByText('item', { exact: true })).toBeVisible()
    await expect(relation.getByText('id', { exact: true })).toBeVisible()
    await relation.getByRole('link', { name: parent, exact: true }).click()
    await expect(page).toHaveURL(new RegExp('/table/' + parent + '\\?view=structure'))
    await expect(page.getByRole('textbox', { name: 'Table name' })).toHaveValue(parent)
  } finally {
    await runSQL(page, `DROP TABLE IF EXISTS ${child}; DROP TABLE IF EXISTS ${parent};`)
  }
})

test('follow a foreign key into another MySQL database or PostgreSQL schema', async ({ page, db, engine, prefix }) => {
  const external = prefix + '_scope'
  const parent = prefix + '_parent'
  const child = prefix + '_child'
  const kind = engine === 'postgres' ? 'SCHEMA' : 'DATABASE'
  await runSQL(page, `CREATE ${kind} ${external}`)
  try {
    await runSQL(page, `CREATE TABLE ${external}.${parent} (id INT PRIMARY KEY);
      CREATE TABLE ${child} (parent_id INT,
      FOREIGN KEY (parent_id) REFERENCES ${external}.${parent}(id))`)
    await page.goto(dbPath(db, 'relationships'))
    const target = page.getByRole('link', { name: external + '.' + parent, exact: true }).first()
    await target.click()
    await expect(page.getByRole('textbox', { name: 'Table name' })).toHaveValue(parent)
    const url = new URL(page.url())
    expect(url.pathname).toBe(dbPath(engine === 'postgres' ? db : external, 'table/' + parent))
    if (engine === 'postgres') expect(url.searchParams.get('schema')).toBe(external)
  } finally {
    await page.goto(dbPath(db))
    await runSQL(page, `DROP TABLE IF EXISTS ${child}; DROP TABLE IF EXISTS ${external}.${parent}; DROP ${kind} ${external}`)
  }
})

import { expect, test } from './fixtures'

test('PostgreSQL role creation, direct privileges and inheritance', async ({ page, engine, prefix, db }) => {
  test.skip(engine !== 'postgres')
  const group = prefix + '_readers'
  const member = prefix + '_member'
  await page.getByRole('link', { name: 'Roles and privileges', exact: true }).click()
  const view = page.locator('.tool-workspace')
  async function create(name: string) {
    await view.getByRole('button', { name: 'Create role', exact: true }).click()
    const form = view.locator('form').filter({ has: page.getByRole('heading', { name: 'Create role' }) })
    await form.getByLabel('Name', { exact: true }).fill(name)
    await form.getByRole('button', { name: 'Create role' }).click()
    await expect(view.getByRole('heading', { name: 'Role settings · ' + name })).toBeVisible()
  }
  const privilege = view.getByRole('row').filter({ has: page.getByRole('cell', { name: 'CREATE', exact: true }) })
  async function drop(name: string) {
    await view.getByRole('button', { name: 'Drop role' }).click()
    await page.getByRole('alertdialog', { name: 'Drop role “' + name + '”?' }).getByRole('button', { name: 'Drop role' }).click()
    await expect(view.getByRole('heading', { name: 'Role settings · ' + name })).toHaveCount(0)
  }
  try {
    await create(group)
    await expect(view.getByRole('combobox', { name: 'Database', exact: true })).toHaveValue(db)
    await privilege.getByRole('button', { name: 'Grant' }).click()
    await expect(privilege).toContainText('Direct')
    await create(member)
    await view.getByRole('combobox', { name: 'Group role', exact: true }).selectOption(group)
    await view.getByRole('button', { name: 'Add membership' }).click()
    await expect(privilege).toContainText('Via ' + group)
    await expect(privilege.getByRole('button', { name: 'Grant' })).toBeVisible()
    await drop(member)
    await view.getByRole('combobox', { name: 'Role', exact: true }).selectOption(group)
    await privilege.getByRole('button', { name: 'Revoke' }).click()
    await expect(privilege).not.toContainText('Direct')
    await drop(group)
  } finally {
    // Clean up only the two uniquely named roles and their test grant.
    const headers = { 'X-Requested-With': 'XMLHttpRequest' }
    await page.request.post('/api/databases/' + encodeURIComponent(db) + '/role-privileges', {
      headers, data: { role: group, object: { kind: 'database' }, privilege: 'CREATE', grant: false },
    })
    for (const name of [member, group]) {
      await page.request.post('/api/roles', { headers, data: { action: 'drop', name } })
    }
  }
})

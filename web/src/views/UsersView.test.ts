import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersView from './UsersView.vue'
import { adminMessages } from '../i18n/admin'

function response(body: unknown, status = 200): Response { return { ok: status < 400, status, json: async () => body } as Response }
const defaultCatalog = ['SELECT','INSERT'].map(name => ({name,levels:['global','database','table']}))
function setup(catalog = defaultCatalog, { user = 'alice', databasePatterns = false } = {}) {
  const fetcher = vi.fn(async (url: string) => response(url === '/api/users' ? { users: [{ user, host: '%', plugin: 'mysql_native_password', limits: { queriesPerHour: 5, updatesPerHour: 0, connectionsPerHour: 0, userConnections: 0 } }] } : url === '/api/users/privileges' ? { privileges: catalog, databasePatterns } : { grants: [{ scope: { level: 'global' }, privileges: ['SELECT'], editable: true, grantOption: false }] }))
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(UsersView, { attachTo: document.body, global: { plugins: [createPinia(), createI18n({ legacy: false, locale: 'en', messages: adminMessages })] } })
  return { wrapper, fetcher }
}
afterEach(() => vi.unstubAllGlobals())
describe('UsersView', () => {
  it('creates an account and clears its password after saving', async () => {
    const { wrapper, fetcher } = setup(); await flushPromises()
    await wrapper.get('[data-test="start-create"]').trigger('click')
    await wrapper.get('[data-test="create-user"]').setValue('new account')
    await wrapper.get('[data-test="create-host"]').setValue('localhost')
    await wrapper.get('[data-test="create-password"]').setValue('secret')
    await wrapper.get('[data-test="create-form"] button').trigger('click'); await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/users', expect.objectContaining({ method: 'POST', body: JSON.stringify({ user: 'new account', host: 'localhost', password: 'secret' }) }))
    await wrapper.get('[data-test="start-create"]').trigger('click')
    expect((wrapper.get('[data-test="create-password"]').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })
  it('loads scoped privileges and revokes an unchecked privilege', async () => {
    const { wrapper, fetcher } = setup(); await flushPromises()
    await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('mysql_native_password')
    const select = wrapper.get('[data-test="privilege-SELECT"]')
    expect((select.element as HTMLInputElement).checked).toBe(true)
    await select.setValue(false)
    await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({ method: 'DELETE', body: JSON.stringify({ scope: { level: 'global' }, privileges: ['SELECT'], grantOption: false }) }))
    wrapper.unmount()
  })
})

it('keeps an ALL PRIVILEGES scope read-only instead of expanding it to a partial catalog', async () => {
 const {wrapper,fetcher}=setup();await flushPromises()
 fetcher.mockImplementation(async (url:string) => response(url.includes('/grants') ? {grants:[{scope:{level:'global'},privileges:['ALL PRIVILEGES'],editable:false,grantOption:false}]} : {users:[]}))
 await wrapper.get('[data-test="select-user"]').trigger('click');await flushPromises()
 expect(wrapper.get('[data-test="save-grants"]').attributes('disabled')).toBeDefined()
 expect(wrapper.text()).toContain('ALL PRIVILEGES')
 wrapper.unmount()
})

it('creates an anonymous account only with its explicit checkbox enabled', async () => {
 const {wrapper,fetcher}=setup();await flushPromises()
 await wrapper.get('[data-test="start-create"]').trigger('click')
 await wrapper.get('[data-test="create-anonymous"]').setValue(true)
 await wrapper.get('[data-test="create-host"]').setValue('localhost')
 await wrapper.get('[data-test="create-form"]').trigger('submit');await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users',expect.objectContaining({method:'POST',body:JSON.stringify({user:'',host:'localhost',password:''})}))
 wrapper.unmount()
})

// Clicking the real button exercises UiButton's native form activation.
it('renames the selected account when its Save button is clicked', async () => {
 const { wrapper, fetcher } = setup(); await flushPromises()
 await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
 await wrapper.get('[data-test="tab-accountSettings"]').trigger('click')
 const form = wrapper.findAll('.editor form')[0]!
 await form.findAll('input')[0]!.setValue('renamed')
 await form.findAll('input')[1]!.setValue('localhost')
 await form.get('button').trigger('click'); await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users/alice?host=%25', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({user:'renamed',host:'localhost'}) }))
 wrapper.unmount()
})
it('changes the selected account password when its Save button is clicked', async () => {
 const { wrapper, fetcher } = setup(); await flushPromises()
 await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
 await wrapper.get('[data-test="tab-accountSettings"]').trigger('click')
 const form = wrapper.findAll('.editor form')[1]!
 await form.get('input').setValue('replacement')
 await form.get('button').trigger('click'); await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users/alice/password?host=%25', expect.objectContaining({ method: 'PUT', body: JSON.stringify({password:'replacement'}) }))
 expect((form.get('input').element as HTMLInputElement).value).toBe('')
 wrapper.unmount()
})
it('saves account limits when its Save button is clicked', async () => {
 const { wrapper, fetcher } = setup(); await flushPromises()
 await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
 await wrapper.get('[data-test="tab-accountSettings"]').trigger('click')
 const form = wrapper.findAll('.editor form')[2]!
 await form.findAll('input')[0]!.setValue(42)
 await form.get('button').trigger('click'); await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users/alice/limits?host=%25', expect.objectContaining({ method: 'PUT', body: JSON.stringify({queriesPerHour:42,updatesPerHour:0,connectionsPerHour:0,userConnections:0}) }))
 wrapper.unmount()
})

it('opens an existing database grant by its exact scope and removes only its grant option', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  fetcher.mockImplementation(async () => response({ grants: [
    { scope: { level: 'global' }, privileges: ['USAGE'], editable: true, grantOption: false },
    { scope: { level: 'database', database: 'odd_% db' }, privileges: ['SELECT', 'INSERT'], editable: true, grantOption: true },
  ] }))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  const open = wrapper.findAll('[data-test="open-scope"]').find(b => b.text().includes('odd_% db'))
  expect(open).toBeDefined()
  await open!.trigger('click')
  expect((wrapper.get('[data-test="privilege-INSERT"]').element as HTMLInputElement).checked).toBe(true)
  const save = wrapper.get('[data-test="save-grants"]')
  expect(save.attributes('disabled')).toBeDefined()
  await wrapper.get('[data-test="grant-option"]').setValue(false)
  expect(wrapper.get('[data-test="pending-grants"]').text()).toContain('GRANT OPTION')
  await save.trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({ method: 'DELETE', body: JSON.stringify({ scope: { level: 'database', database: 'odd_% db' }, privileges: [], grantOption: true }) }))
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ method: 'POST' }))
  wrapper.unmount()
})

it('blocks privilege writes when the server fails to read grants and supports retry', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  fetcher.mockImplementationOnce(async () => response({ error: { message: 'Cannot read grants' } }, 403))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  expect(wrapper.get('[role=alert]').text()).toContain('Cannot read grants')
  expect(wrapper.find('[data-test="privilege-SELECT"]').exists()).toBe(false)
  await wrapper.get('[data-test="retry-grants"]').trigger('click'); await flushPromises()
  expect((wrapper.get('[data-test="privilege-SELECT"]').element as HTMLInputElement).checked).toBe(true)
  wrapper.unmount()
})

it('uses database and table suggestions to grant access only to the chosen table', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  fetcher.mockImplementation(async (url:string) => {
    if (url === '/api/databases') return response({ databases: [{ name: 'odd/name' }] })
    if (url === '/api/databases/odd%2Fname/tables') return response({ tables: [{ name: 'sales_items' }] })
    return response({ grants: [] })
  })
  await wrapper.get('[data-test="choose-scope"]').trigger('click'); await flushPromises()
  await wrapper.get('[data-test="scope-level"]').setValue('table')
  expect(wrapper.findAll('#grant-databases option').map(o => o.attributes('value'))).toContain('odd/name')
  await wrapper.get('[data-test="scope-database"]').setValue('odd/name'); await flushPromises()
  expect(wrapper.findAll('#grant-tables option').map(o => o.attributes('value'))).toContain('sales_items')
  await wrapper.get('[data-test="scope-table"]').setValue('sales_items')
  await wrapper.get('[data-test="open-new-scope"]').trigger('click')
  await wrapper.get('[data-test="privilege-SELECT"]').setValue(true)
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({ method: 'POST', body: JSON.stringify({ scope: { level: 'table', database: 'odd/name', table: 'sales_items' }, privileges: ['SELECT'], grantOption: false }) }))
  wrapper.unmount()
})

it('finds an account by host and opens the exact newly created account for database access', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  await wrapper.get('[data-test="find-account"]').setValue('unknown')
  expect(wrapper.find('[data-test="select-user"]').exists()).toBe(false)
  await wrapper.get('[data-test="find-account"]').setValue('%')
  expect(wrapper.find('[data-test="select-user"]').exists()).toBe(true)
  await wrapper.get('[data-test="start-create"]').trigger('click')
  await wrapper.get('[data-test="create-user"]').setValue('alice')
  await wrapper.get('[data-test="create-host"]').setValue('localhost')
  const limits = { queriesPerHour: 0, updatesPerHour: 0, connectionsPerHour: 0, userConnections: 0 }
  fetcher.mockImplementation(async (url:string) => {
    if (url === '/api/users') return response({ users: ['%', 'localhost'].map(host => ({ user: 'alice', host, plugin: '', limits })) })
    if (url === '/api/databases') return response({ databases: [] })
    return response({ grants: [] })
  })
  await wrapper.get('[data-test="create-form"]').trigger('submit'); await flushPromises()
  expect(wrapper.get('.identity').text()).toContain('alice@localhost')
  expect(wrapper.get('[data-test="scope-level"]').element).toHaveProperty('value', 'database')
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=localhost', expect.objectContaining({method:'GET'}))
  wrapper.unmount()
})

it('grants a database privilege for the literal username followed by an underscore prefix', async () => {
  const { wrapper, fetcher } = setup([...defaultCatalog, { name: 'PROCESS', levels: ['global'] }], {
    user: 'r_%\\d', databasePatterns: true,
  })
  await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  const shortcut = wrapper.find('[data-test="username-database-pattern"]')
  expect(shortcut.exists()).toBe(true)
  await shortcut.trigger('click')
  const database = 'r\\_\\%\\\\d\\_%'
  expect(wrapper.get('[data-test="scope-database-pattern"]').element).toHaveProperty('value', database)
  expect(wrapper.text()).toContain('future databases')
  await wrapper.get('[data-test="open-new-scope"]').trigger('click')
  expect(wrapper.find('[data-test="privilege-PROCESS"]').exists()).toBe(false)
  await wrapper.get('[data-test="privilege-SELECT"]').setValue(true)
  await wrapper.get('[data-test="save-grants"]').trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/r_%25%5Cd/grants?host=%25', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ scope: { level: 'databasePattern', database }, privileges: ['SELECT'], grantOption: false }),
  }))
  wrapper.unmount()
})

it('requires an unescaped wildcard, allows a custom pattern, and reads back the same scope for revocation', async () => {
  const { wrapper, fetcher } = setup(defaultCatalog, { databasePatterns: true })
  await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  const level = wrapper.get('[data-test="scope-level"]')
  expect(level.find('option[value="databasePattern"]').exists()).toBe(true)
  await level.setValue('databasePattern')
  const input = wrapper.get('[data-test="scope-database-pattern"]')
  const open = wrapper.get('[data-test="open-new-scope"]')
  for (const invalid of ['', 'sales\\_%\\', 'sales\\%', 'sales\\q%', 'sales\0%']) {
    await input.setValue(invalid)
    expect(open.attributes('disabled')).toBeDefined()
    await wrapper.get('.scope-picker').trigger('submit')
    expect(wrapper.find('[data-test="save-grants"]').exists()).toBe(false)
  }
  const scope = { level: 'databasePattern', database: 'sales\\_%_archive' }
  await input.setValue(scope.database)
  await open.trigger('click')
  await wrapper.get('[data-test="privilege-INSERT"]').setValue(true)
  fetcher.mockResolvedValueOnce(response(null, 204)).mockResolvedValueOnce(response({ grants: [
    { scope, privileges: ['INSERT'], grantOption: false, editable: true },
  ] }))
  await wrapper.get('[data-test="save-grants"]').trigger('click')
  await flushPromises()
  const openGrant = wrapper.get('[data-test="open-scope"]')
  expect(openGrant.text()).toContain(scope.database)
  await openGrant.trigger('click')
  expect(wrapper.get('[data-test="privilege-INSERT"]').element).toHaveProperty('checked', true)
  await wrapper.get('[data-test="privilege-INSERT"]').setValue(false)
  await wrapper.get('[data-test="save-grants"]').trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({
    method: 'DELETE', body: JSON.stringify({ scope, privileges: ['INSERT'], grantOption: false }),
  }))
  wrapper.unmount()
})

it('offers matching existing tables even when persistent database patterns are unavailable', async () => {
  const { wrapper } = setup()
  await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  const level = wrapper.get('[data-test="scope-level"]')
  expect(level.find('option[value="tablePattern"]').exists()).toBe(true)
  expect(level.find('option[value="databasePattern"]').exists()).toBe(false)
  expect(wrapper.find('[data-test="username-database-pattern"]').exists()).toBe(false)
  wrapper.unmount()
})

it('keeps the table matcher open during readback and guards navigation while its grants are staged or in flight', async () => {
  const { wrapper, fetcher } = setup()
  await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click')
  await flushPromises()
  fetcher.mockImplementation(async (url: string) => response(url === '/api/databases'
    ? { databases: [{ name: 'warehouse' }] } : { tables: [{ name: 'april' }] }))
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  await wrapper.get('[data-test="scope-level"]').setValue('tablePattern')
  await wrapper.get('[data-test="scope-database"]').setValue('warehouse')
  await wrapper.get('[data-test="open-new-scope"]').trigger('click')
  await flushPromises()
  const pattern = wrapper.find('[data-test="table-pattern"]')
  expect(pattern.exists()).toBe(true)
  await pattern.setValue('apr%')
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  vi.stubGlobal('confirm', vi.fn(() => false))
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  expect(window.confirm).toHaveBeenCalled()
  expect(wrapper.find('[data-test="scope-level"]').exists()).toBe(false)
  let finishWrite!: (value: Response) => void
  let finishRead!: (value: Response) => void
  fetcher.mockImplementationOnce(() => new Promise<Response>(resolve => { finishWrite = resolve }))
    .mockImplementationOnce(() => new Promise<Response>(resolve => { finishRead = resolve }))
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  expect(wrapper.get('[data-test="accounts-list"]').attributes('disabled')).toBeDefined()
  finishWrite(response(null, 204))
  await flushPromises()
  expect(wrapper.get('[data-test="choose-scope"]').attributes('disabled')).toBeDefined()
  finishRead(response({ error: { message: 'Grant list unavailable' } }, 503))
  await flushPromises()
  expect(wrapper.get('[data-test="table-grant-result"]').text()).toContain('1')
  expect(wrapper.get('[role="alert"]').text()).toContain('Grant list unavailable')
  fetcher.mockResolvedValueOnce(response({ grants: [
    { scope: { level: 'table', database: 'warehouse', table: 'april' }, privileges: ['SELECT'], editable: true, grantOption: false },
  ] }))
  await wrapper.get('[data-test="retry-grants"]').trigger('click')
  await flushPromises()
  expect(wrapper.get('[data-test="open-scope"]').text()).toContain('warehouse.april')
  expect(wrapper.find('[data-test="table-pattern"]').exists()).toBe(true)
  expect(wrapper.get('[data-test="accounts-list"]').attributes('disabled')).toBeUndefined()
  wrapper.unmount()
})

it('stages all database privileges independently of filter and delegation on a database pattern', async () => {
  const dbPrivileges = ['SELECT', 'CREATE', 'EXECUTE']
  const catalog = [
    ...dbPrivileges.map(name => ({ name, levels: ['global', 'database'] })),
    { name: 'PROCESS', levels: ['global'] },
  ]
  const { wrapper, fetcher } = setup(catalog, { databasePatterns: true })
  await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="choose-scope"]').trigger('click')
  await wrapper.get('[data-test="username-database-pattern"]').trigger('click')
  await wrapper.get('[data-test="open-new-scope"]').trigger('click')
  await wrapper.get('input[type="search"]').setValue('SELECT')
  const all = wrapper.find('[data-test="toggle-database-pattern-privileges"]')
  expect(all.exists()).toBe(true)
  await all.setValue(true)
  expect(wrapper.get('[data-test="pending-grants"]').text()).toContain('EXECUTE')
  expect(wrapper.get('[data-test="grant-option"]').element).toHaveProperty('checked', false)
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ method: 'POST' }))
  await all.setValue(false)
  expect(wrapper.get('[data-test="save-grants"]').attributes('disabled')).toBeDefined()
  await all.setValue(true)
  await wrapper.get('[data-test="save-grants"]').trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ scope: { level: 'databasePattern', database: 'alice\\_%' }, privileges: dbPrivileges, grantOption: false }),
  }))
  wrapper.unmount()
})

it('reads actual grants after a partial write failure before allowing a retry', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  await wrapper.get('[data-test="privilege-SELECT"]').setValue(false)
  await wrapper.get('[data-test="privilege-INSERT"]').setValue(true)
  fetcher.mockResolvedValueOnce(response(null, 204))
    .mockResolvedValueOnce(response({error:{message:'Write denied'}},403))
    .mockResolvedValueOnce(response({grants:[]}))
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('Write denied')
  expect(wrapper.get('[data-test="privilege-SELECT"]').element).toHaveProperty('checked',false)
  expect(wrapper.get('[data-test="privilege-INSERT"]').element).toHaveProperty('checked',false)
  expect(wrapper.get('[data-test="save-grants"]').attributes('disabled')).toBeDefined()
  wrapper.unmount()
})

it('edits a single privilege without extending delegation across separate global grants', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  fetcher.mockImplementation(async () => response({ grants: [
    { scope: { level: 'global' }, privileges: ['SELECT'], editable: true, grantOption: false },
    { scope: { level: 'global' }, privileges: ['INSERT'], editable: true, grantOption: true },
  ] }))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  expect(wrapper.find('[data-test="privilege-SELECT"]').exists()).toBe(true)
  expect(wrapper.find('[data-test="grant-option"]').exists()).toBe(false)
  await wrapper.get('[data-test="privilege-SELECT"]').setValue(false)
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({ method: 'DELETE', body: JSON.stringify({ scope: { level: 'global' }, privileges: ['SELECT'], grantOption: false }) }))
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ method: 'POST' }))
  wrapper.unmount()
})

it('grants delegation without resending already granted privileges', async () => {
  const { wrapper, fetcher } = setup(); await flushPromises()
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  await wrapper.get('[data-test="grant-option"]').setValue(true)
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({ method: 'POST', body: JSON.stringify({ scope: { level: 'global' }, privileges: [], grantOption: true }) }))
  wrapper.unmount()
})

it('stages only the displayed data privileges in the selected scope', async () => {
  const tableRights = ['SELECT', 'UPDATE', 'INSERT', 'INDEX', 'CREATE', 'LOCK TABLES'].map(name => ({name, levels:['global','table']}))
  const { wrapper, fetcher } = setup([...tableRights, {name:'DELETE',levels:['global']}])
  await flushPromises()
  const scope = { level: 'table', database: 'shop', table: 'items' }
  fetcher.mockImplementation(async () => response({ grants: [
    { scope, privileges: ['SELECT','INDEX'], editable:true, grantOption:false },
  ] }))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  await wrapper.get('[data-test="open-scope"]').trigger('click')
  const dml = wrapper.find('[data-grant-group="data"]')
  expect(dml.exists()).toBe(true)
  expect(dml.find('[data-test="privilege-DELETE"]').exists()).toBe(false)
  expect(wrapper.get('[data-grant-group="structure"]').text()).toContain('INDEX')
  expect(wrapper.get('[data-grant-group="administration"]').text()).toContain('LOCK TABLES')
  const toggle = wrapper.get('[data-test="toggle-shown-data"]')
  expect(toggle.element).toHaveProperty('indeterminate', true)
  await wrapper.get('input[type="search"]').setValue('in')
  await toggle.setValue(true)
  expect(wrapper.get('[data-test="pending-grants"]').text()).toContain('INSERT')
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(),expect.objectContaining({method:'POST'}))
  await wrapper.get('input[type="search"]').setValue('')
  expect(wrapper.get('[data-test="privilege-UPDATE"]').element).toHaveProperty('checked',false)
  expect(wrapper.get('[data-test="privilege-SELECT"]').element).toHaveProperty('checked',true)
  expect(wrapper.get('[data-test="privilege-INDEX"]').element).toHaveProperty('checked',true)
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({method:'POST',body:JSON.stringify({scope,privileges:['INSERT'],grantOption:false})}))
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(),expect.objectContaining({method:'DELETE'}))
  wrapper.unmount()
})

it('clears only shown data privileges and leaves filtered grants intact', async () => {
  const { wrapper, fetcher } = setup()
  await flushPromises()
  fetcher.mockImplementation(async () => response({grants:[{scope:{level:'global'},privileges:['SELECT','INSERT'],editable:true,grantOption:false}]}))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  await wrapper.get('input[type="search"]').setValue('sel')
  const toggle = wrapper.find('[data-test="toggle-shown-data"]')
  expect(toggle.exists()).toBe(true)
  expect(toggle.element).toHaveProperty('checked',true)
  await toggle.setValue(false)
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({method:'DELETE',body:JSON.stringify({scope:{level:'global'},privileges:['SELECT'],grantOption:false})}))
  wrapper.unmount()
})

it('adds BACKUP_ADMIN without copying the grant option of an existing static global grant', async () => {
  const { wrapper, fetcher } = setup([...defaultCatalog, {name:'BACKUP_ADMIN', levels:['global']}])
  await flushPromises()
  fetcher.mockImplementation(async () => response({grants:[
    {scope:{level:'global'}, privileges:['SELECT'], editable:true, grantOption:true},
  ]}))
  await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
  expect(wrapper.get('[data-test="grant-option"]').element).toHaveProperty('checked',true)
  await wrapper.get('[data-test="privilege-BACKUP_ADMIN"]').setValue(true)
  const pending = wrapper.get('[data-test="pending-grants"]').text()
  expect(pending).toContain('BACKUP_ADMIN')
  expect(pending).not.toContain('GRANT OPTION')
  await wrapper.get('[data-test="save-grants"]').trigger('click'); await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/users/alice/grants?host=%25', expect.objectContaining({
    method:'POST', body:JSON.stringify({scope:{level:'global'}, privileges:['BACKUP_ADMIN'], grantOption:false}),
  }))
  expect(fetcher).not.toHaveBeenCalledWith(expect.anything(),expect.objectContaining({method:'DELETE'}))
  wrapper.unmount()
})

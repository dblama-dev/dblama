import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersView from './UsersView.vue'
import { adminMessages } from '../i18n/admin'

function response(body: unknown, status = 200): Response { return { ok: status < 400, status, json: async () => body } as Response }
function setup() {
  const fetcher = vi.fn(async (url: string) => response(url === '/api/users' ? { users: [{ user: 'alice', host: '%', plugin: 'mysql_native_password', limits: { queriesPerHour: 5, updatesPerHour: 0, connectionsPerHour: 0, userConnections: 0 } }] } : url === '/api/users/privileges' ? { privileges: [{ name: 'SELECT', levels: ['global', 'database', 'table'] }, { name: 'INSERT', levels: ['global', 'database', 'table'] }] } : { grants: [{ scope: { level: 'global' }, privileges: ['SELECT'], editable: true, grantOption: false }] }))
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(UsersView, { attachTo: document.body, global: { plugins: [createPinia(), createI18n({ legacy: false, locale: 'en', messages: adminMessages })] } })
  return { wrapper, fetcher }
}
afterEach(() => vi.unstubAllGlobals())
describe('UsersView', () => {
  it('creates an account and clears its password after saving', async () => {
    const { wrapper, fetcher } = setup(); await flushPromises()
    await wrapper.get('[data-test="create-user"]').setValue('new account')
    await wrapper.get('[data-test="create-host"]').setValue('localhost')
    await wrapper.get('[data-test="create-password"]').setValue('secret')
    await wrapper.get('[data-test="create-form"] button').trigger('click'); await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/users', expect.objectContaining({ method: 'POST', body: JSON.stringify({ user: 'new account', host: 'localhost', password: 'secret' }) }))
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
 await wrapper.get('[data-test="create-anonymous"]').setValue(true)
 await wrapper.get('[data-test="create-host"]').setValue('localhost')
 await wrapper.get('[data-test="create-form"]').trigger('submit');await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users',expect.objectContaining({method:'POST',body:JSON.stringify({user:'',host:'localhost',password:''})}))
 wrapper.unmount()
})

// Clicking the real button exercises NButton's native form activation.
it('renames the selected account when its Save button is clicked', async () => {
 const { wrapper, fetcher } = setup(); await flushPromises()
 await wrapper.get('[data-test="select-user"]').trigger('click'); await flushPromises()
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
 const form = wrapper.findAll('.editor form')[2]!
 await form.findAll('input')[0]!.setValue(42)
 await form.get('button').trigger('click'); await flushPromises()
 expect(fetcher).toHaveBeenCalledWith('/api/users/alice/limits?host=%25', expect.objectContaining({ method: 'PUT', body: JSON.stringify({queriesPerHour:42,updatesPerHour:0,connectionsPerHour:0,userConnections:0}) }))
 wrapper.unmount()
})

import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, expect, it, vi } from 'vitest'
import TablePatternGrants from './TablePatternGrants.vue'

const path = '/api/users/a_%25/grants?host=%25'
const database = 'odd/db_%'
function table(name: string, isView = false) {
  return { name, isView, rows: 0, sizeBytes: 0, engine: 'InnoDB', comment: '' }
}
function response(payload: unknown, status = 200) {
  return { ok: status < 400, status, json: async () => payload } as Response
}
function setup(fetcher: (url: string, request: RequestInit) => Promise<Response>, username = 'a_%') {
  vi.stubGlobal('fetch', fetcher)
  return mount(TablePatternGrants, {
    props: {
      path, database, username,
      catalog: [
        { name: 'SELECT', levels: ['global', 'database', 'table'] },
        { name: 'CREATE USER', levels: ['global'] },
      ],
    },
    global: { plugins: [createI18n({ legacy: false, locale: 'en' })] },
  })
}
afterEach(() => vi.unstubAllGlobals())

it('previews only existing matching tables and grants additive exact scopes on request', async () => {
  const tables = [table('a_%_sales'), table('a_%_view', true), table('abc_sales'), table('A_%_sales'), table('a_%_🪐')]
  const fetcher = vi.fn(async (_url: string, request: RequestInit) =>
    request.method === 'GET' ? response({ tables }) : response(null, 204))
  const wrapper = setup(fetcher)
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith('/api/databases/odd%2Fdb_%25/tables', expect.objectContaining({ method: 'GET' }))
  expect(wrapper.find('[data-test="table-privilege-CREATE USER"]').exists()).toBe(false)
  await wrapper.get('[data-test="table-prefix"]').trigger('click')
  expect(wrapper.get('[data-test="table-pattern"]').element).toHaveProperty('value', 'a\\_\\%\\_%')
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  expect(wrapper.get('[data-test="table-matches"]').findAll('li').map(li => li.text())).toEqual(['a_%_sales', 'a_%_🪐'])
  expect(fetcher.mock.calls).toHaveLength(1)
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  await flushPromises()
  const writes = fetcher.mock.calls.slice(1).map(([url, request]) => [url, request.method, JSON.parse(String(request.body))])
  expect(writes).toEqual(['a_%_sales', 'a_%_🪐'].map(name => [path, 'POST', {
    scope: { level: 'table', database, table: name }, privileges: ['SELECT'], grantOption: false,
  }]))
  expect(wrapper.emitted('changed')).toHaveLength(1)
  expect(wrapper.emitted('dirty')?.at(-1)).toEqual([false])
  expect(wrapper.get('[data-test="table-grant-result"]').text()).toContain('2')
  wrapper.unmount()
})

it('locks the preview during writes and retries only failed names from the original batch', async () => {
  let finish!: (response: Response) => void
  const pending = new Promise<Response>(resolve => { finish = resolve })
  const attempted: string[] = []
  const wrapper = setup(async (_url, request) => {
    if (request.method === 'GET') return response({ tables: [table('one'), table('two'), table('three')] })
    const name = JSON.parse(String(request.body)).scope.table
    attempted.push(name)
    if (attempted.length === 1) return pending
    if (attempted.length === 2) return response({ error: { message: 'access denied' } }, 403)
    return response(null, 204)
  })
  await flushPromises()
  await wrapper.get('[data-test="table-pattern"]').setValue('%')
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  expect(wrapper.emitted('busy')?.at(-1)).toEqual([true])
  expect(wrapper.get('[data-test="table-pattern"]').element).toHaveProperty('disabled', true)
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  expect(attempted).toEqual(['one'])
  finish(response(null, 204))
  await flushPromises()
  expect(attempted).toEqual(['one', 'two', 'three'])
  expect(wrapper.emitted('busy')?.at(-1)).toEqual([false])
  expect(wrapper.emitted('changed')).toHaveLength(1)
  expect(wrapper.get('[role="alert"]').text()).toContain('two')
  expect(wrapper.get('[role="alert"]').text()).toContain('access denied')
  await wrapper.get('[data-test="retry-table-grants"]').trigger('click')
  await flushPromises()
  expect(attempted).toEqual(['one', 'two', 'three', 'two'])
  expect(wrapper.find('[data-test="retry-table-grants"]').exists()).toBe(false)
  expect(wrapper.get('[data-test="table-grant-result"]').text()).toContain('3')
  expect(wrapper.emitted('changed')).toHaveLength(2)
  wrapper.unmount()
})

it('allows retry of a metadata error and blocks empty, invalid and unmatched masks', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce(response({ error: { message: 'unavailable' } }, 503))
    .mockResolvedValue(response({ tables: [table('one')] }))
  const wrapper = setup(fetcher, '')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('unavailable')
  expect(wrapper.find('[data-test="table-prefix"]').exists()).toBe(false)
  await wrapper.get('[data-test="retry-table-list"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  for (const mask of ['', '%\\q', 'different%']) {
    await wrapper.get('[data-test="table-pattern"]').setValue(mask)
    expect(wrapper.get('[data-test="apply-table-grants"]').attributes('disabled')).toBeDefined()
  }
  expect(fetcher.mock.calls.map(([, req]) => req.method)).toEqual(['GET', 'GET'])
  wrapper.unmount()
})

it('stops sending grants after leaving the account while a write is in flight', async () => {
  let finish!: (response: Response) => void
  const pending = new Promise<Response>(resolve => { finish = resolve })
  const writes: string[] = []
  const wrapper = setup(async (_url, request) => {
    if (request.method === 'GET') return response({ tables: [table('one'), table('two')] })
    writes.push(JSON.parse(String(request.body)).scope.table)
    return pending
  })
  await flushPromises()
  await wrapper.get('[data-test="table-pattern"]').setValue('%')
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  wrapper.unmount()
  finish(response(null, 204))
  await flushPromises()
  expect(writes).toEqual(['one'])
  expect(wrapper.emitted('changed')).toBeUndefined()
})

it('requests fresh grants when every write loses its response and keeps the failed names for retry', async () => {
  const wrapper = setup(async (_url, request) => {
    if (request.method === 'GET') return response({ tables: [table('one'), table('two')] })
    throw new TypeError('connection lost')
  })
  await flushPromises()
  await wrapper.get('[data-test="table-pattern"]').setValue('%')
  await wrapper.get('[data-test="table-privilege-SELECT"]').setValue(true)
  await wrapper.get('[data-test="apply-table-grants"]').trigger('click')
  await flushPromises()
  expect(wrapper.get('[data-test="table-grant-result"] [role="status"]').text()).toContain('0 of 2')
  expect(wrapper.get('[role="alert"]').findAll('li code').map(name => name.text())).toEqual(['one', 'two'])
  expect(wrapper.find('[data-test="retry-table-grants"]').exists()).toBe(true)
  expect(wrapper.emitted('changed')).toHaveLength(1)
  wrapper.unmount()
})

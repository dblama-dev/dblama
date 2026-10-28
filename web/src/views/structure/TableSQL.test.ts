import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { useCatalogStore } from '../../stores/catalog'
import i18n from '../../i18n'
import TableSQL from './TableSQL.vue'

vi.mock('../../api/client', () => ({ api: vi.fn() }))
beforeEach(() => { setActivePinia(createPinia()); vi.mocked(api).mockReset() })
function open() {
  return mount(TableSQL, { props: { db: 'app db', table: 'Mixed%Case' }, global: { plugins: [i18n] } })
}
it('shows escaped DDL for encoded identifiers in the selected schema and refreshes it', async () => {
  const catalog = useCatalogStore(); catalog.schemaAware = true; catalog.schema = 'Odd Schema'
  vi.mocked(api).mockResolvedValueOnce({ sql: 'CREATE TABLE "<img src=x onerror=alert(1)>" (id int)' }).mockResolvedValueOnce({ sql: 'CREATE TABLE second (id int)' })
  const wrapper = open(); await flushPromises()
  expect(api).toHaveBeenCalledWith('GET', '/api/databases/app%20db/tables/Mixed%25Case/create-sql?schema=Odd%20Schema')
  expect(wrapper.get('pre code').text()).toContain('<img src=x onerror=alert(1)>')
  expect(wrapper.find('img').exists()).toBe(false)
  await wrapper.get('[data-testid=sql-refresh]').trigger('click'); await flushPromises()
  expect(wrapper.get('pre code').text()).toBe('CREATE TABLE second (id int)')
  wrapper.unmount()
})
it('shows a failed load and recovers on refresh', async () => {
  vi.mocked(api).mockRejectedValueOnce(new Error('permission denied')).mockResolvedValueOnce({ sql: 'CREATE TABLE allowed (id int)' })
  const wrapper = open(); await flushPromises()
  expect(wrapper.text()).toContain('permission denied')
  expect(wrapper.find('pre').exists()).toBe(false)
  await wrapper.get('[data-testid=sql-refresh]').trigger('click'); await flushPromises()
  expect(wrapper.text()).not.toContain('permission denied')
  expect(wrapper.get('pre code').text()).toContain('allowed')
  wrapper.unmount()
})
it.each(['resolve', 'reject'])('ignores a late %s after a schema change', async (outcome) => {
  let finish!: (v: { sql: string }) => void
  let fail!: (e: Error) => void
  vi.mocked(api).mockImplementationOnce(() => new Promise((resolve, reject) => { finish = resolve; fail = reject })).mockResolvedValueOnce({ sql: 'CREATE TABLE fresh (id int)' })
  const catalog = useCatalogStore(); catalog.schemaAware = true; catalog.schema = 'old'
  const wrapper = open(); await flushPromises()
  catalog.schema = 'new'; await flushPromises()
  expect(wrapper.get('pre code').text()).toContain('fresh')
  if (outcome === 'resolve') finish({ sql: 'CREATE TABLE stale (id int)' })
  else fail(new Error('stale error'))
  await flushPromises()
  expect(wrapper.get('pre code').text()).toContain('fresh')
  expect(wrapper.text()).not.toContain('stale')
  wrapper.unmount()
})
it('reloads when the addressed table changes', async () => {
  vi.mocked(api).mockResolvedValueOnce({ sql: 'CREATE TABLE first (id int)' }).mockResolvedValueOnce({ sql: 'CREATE TABLE next (id int)' })
  const wrapper = open(); await flushPromises()
  await wrapper.setProps({ db: 'other', table: 'next' }); await flushPromises()
  expect(api).toHaveBeenLastCalledWith('GET', '/api/databases/other/tables/next/create-sql')
  expect(wrapper.get('pre code').text()).toContain('next')
  wrapper.unmount()
})

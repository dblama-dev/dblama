import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { NNotificationProvider } from 'naive-ui'
import { expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import i18n from '../i18n'
import TableView from './TableView.vue'
import DataGrid from '../components/DataGrid.vue'
import RowModal from '../components/RowModal.vue'
vi.mock('../api/client', () => ({ api: vi.fn(), ApiError: class extends Error {} }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { db: 'app', t: 'items' }, query: {} }), useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }))
it('scopes rows, keys and writes to the schema and reloads when it changes', async () => {
  const pinia = createPinia(); setActivePinia(pinia)
  const catalog = useCatalogStore(); catalog.schemaAware = true; catalog.schema = 'Odd Schema'
  vi.mocked(api).mockImplementation(async (_method, path) => {
    if (path.includes('/pk')) return { columns: ['id'] }
    if (path.includes('/rows')) return { columns: [], rows: [], totalRows: 0 }
    return { columns: [], indexes: [], foreignKeys: [], options: {} }
  })
  const wrapper = mount(defineComponent({ setup: () => () => h(NNotificationProvider, null, { default: () => h(TableView) }) }), { global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true } } })
  await flushPromises()
  expect(api).toHaveBeenCalledWith('GET', '/api/databases/app/tables/items/pk?schema=Odd%20Schema')
  expect(api).toHaveBeenCalledWith('GET', expect.stringMatching(/\/rows\?.*schema=Odd%20Schema/))
  expect(wrapper.findComponent(RowModal).props('rowsPath')).toBe('/api/databases/app/tables/items/rows?schema=Odd%20Schema')
  wrapper.findComponent(DataGrid).vm.$emit('delete-rows', [{ id: 1 }])
  await flushPromises()
  expect(api).toHaveBeenCalledWith('DELETE', '/api/databases/app/tables/items/rows?schema=Odd%20Schema', { where: [{ id: 1 }] })
  catalog.schema = 'second'
  await flushPromises()
  expect(api).toHaveBeenCalledWith('GET', '/api/databases/app/tables/items/pk?schema=second')
  wrapper.unmount()
})

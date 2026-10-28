import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, reactive } from 'vue'
import { NNotificationProvider, NTabs } from 'naive-ui'
import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import i18n from '../i18n'
import TableView from './TableView.vue'
import DataGrid from '../components/DataGrid.vue'
import RowModal from '../components/RowModal.vue'
vi.mock('../api/client', () => ({ api: vi.fn(), ApiError: class extends Error {} }))
let route = reactive({ params: { db: 'app', t: 'items' }, query: {} as Record<string, string> })
const push = vi.fn()
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ push, replace: vi.fn() }) }))
beforeEach(() => { route = reactive({ params: { db: 'app', t: 'items' }, query: {} }); vi.mocked(api).mockReset(); push.mockClear() })
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

function openTable() {
 const pinia = createPinia(); setActivePinia(pinia)
 vi.mocked(api).mockImplementation(async (_method, path) => {
  if (path.includes('/create-sql')) return { sql: 'CREATE TABLE visible (id int)' }
  if (path.includes('/pk')) return { columns: ['id'] }
  if (path.includes('/rows')) return { columns: [], rows: [], totalRows: 0 }
  return { columns: [], indexes: [], foreignKeys: [], options: {} }
 })
 return mount(defineComponent({ setup: () => () => h(NNotificationProvider, null, { default: () => h(TableView) }) }), { global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true, StructureEditor: true } } })
}
it('opens SQL from the URL and retains schema when changing sections', async () => {
 route.query = { view: 'sql', schema: 'Odd Schema' }
 const wrapper = openTable(); await flushPromises()
 expect(wrapper.get('pre code').text()).toContain('CREATE TABLE visible')
 wrapper.findComponent(NTabs).vm.$emit('update:value', 'structure'); await flushPromises()
 expect(push).toHaveBeenCalledWith({ query: { view: 'structure', schema: 'Odd Schema' } })
 wrapper.unmount()
})
it('shows structure for a new table addressed by view=sql without requesting DDL', async () => {
 route.params.t = '_new'; route.query = { view: 'sql' }
 const wrapper = openTable(); await flushPromises()
 expect(wrapper.find('pre').exists()).toBe(false)
 expect(wrapper.find('structure-editor-stub').exists()).toBe(true)
 expect(api).not.toHaveBeenCalled()
 wrapper.unmount()
})
it('reads fresh SQL after returning from the structure section', async () => {
 route.query = { view: 'sql' }
 const wrapper = openTable(); await flushPromises()
 route.query = { view: 'structure' }; await flushPromises()
 vi.mocked(api).mockResolvedValueOnce({ sql: 'CREATE TABLE updated (id int, name text)' })
 route.query = { view: 'sql' }; await flushPromises()
 expect(wrapper.get('pre code').text()).toContain('name text')
 wrapper.unmount()
})

it('opens a copied row as a new insert with generated identity cleared', async () => {
 const wrapper = openTable(); await flushPromises()
 vi.mocked(api).mockResolvedValueOnce({ columns: [{name:'id',type:'int',nullable:false,autoIncrement:true},{name:'name',type:'text',nullable:false}], indexes:[],foreignKeys:[],options:{} })
 wrapper.findComponent(DataGrid).vm.$emit('copy-row',{id:1,name:'copied',__pk:{id:1}})
 await flushPromises()
 expect(wrapper.findComponent(RowModal).props('mode')).toBe('insert')
 expect(wrapper.findComponent(RowModal).props('row')).toEqual({name:'copied'})
 wrapper.unmount()
})

it('ignores an old table definition failure after switching schemas during copy', async () => {
 const wrapper = openTable(); await flushPromises()
 const catalog = useCatalogStore()
 catalog.schemaAware = true; catalog.schema = 'old'
 let rejectOld!: (error: Error) => void
 vi.mocked(api).mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectOld = reject }))
 wrapper.findComponent(DataGrid).vm.$emit('copy-row',{id:1,name:'old',__pk:{id:1}})
 await flushPromises()
 catalog.schema = 'new'
 rejectOld(new Error('old schema no longer available'))
 await flushPromises()
 expect(wrapper.findComponent(RowModal).props('show')).toBe(false)
 vi.mocked(api).mockResolvedValueOnce({ columns:[{name:'name',type:'text',nullable:false}],indexes:[],foreignKeys:[],options:{} })
 wrapper.findComponent(DataGrid).vm.$emit('copy-row',{id:2,name:'new',__pk:{id:2}})
 await flushPromises()
 expect(wrapper.findComponent(RowModal).props('row')).toEqual({name:'new'})
 wrapper.unmount()
})

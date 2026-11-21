import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, reactive } from 'vue'
import { UiNotificationProvider } from '../ui'
import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import i18n from '../i18n'
import TableView from './TableView.vue'
import DataGrid from '../components/DataGrid.vue'
import RowModal from '../components/RowModal.vue'
import BulkEdit from '../components/BulkEdit.vue'
import * as download from '../lib/download'
vi.mock('../api/client', () => ({ api: vi.fn(), ApiError: class extends Error {} }))
let route = reactive({ params: { db: 'app', t: 'items' }, query: {} as Record<string, string> })
const push = vi.fn()
vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => route,
  useRouter: () => ({ push, replace: vi.fn() }),
}))
beforeEach(() => { route = reactive({ params: { db: 'app', t: 'items' }, query: {} }); vi.mocked(api).mockReset(); push.mockReset() })
it('scopes rows, keys and writes to the schema and reloads when it changes', async () => {
  const pinia = createPinia(); setActivePinia(pinia)
  const catalog = useCatalogStore(); catalog.schemaAware = true; catalog.schema = 'Odd Schema'
  vi.mocked(api).mockImplementation(async (_method, path) => {
    if (/\/tables(?:\?|$)/.test(path)) return { tables: [] }
    if (path.includes('/pk')) return { columns: ['id'] }
    if (path.includes('/rows')) return { columns: [], rows: [], totalRows: 0 }
    return { columns: [], indexes: [], foreignKeys: [], options: {} }
  })
  const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(TableView) }) }), { global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true } } })
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

it.each(['data', 'structure'])('refreshes table %s metadata when the database and schema change', async view => {
  const pinia = createPinia()
  useSessionStore(pinia).sessionInfo = {
    driver: { name: 'PostgreSQL', flavor: 'postgres', version: '17', host: 'localhost', user: 'u' },
    caps: {} as never,
    database: null,
  }
  const catalog = useCatalogStore(pinia)
  catalog.schemaAware = true
  catalog.schema = 'public'
  route.query = { view }
  vi.mocked(api).mockImplementation(async (_method, path) => {
    if (path.includes('/metadata')) return { columnTypes: [], operators: [], collations: [] }
    if (/\/tables(?:\?|$)/.test(path)) return { tables: [] }
    if (path.includes('/rows')) return { columns: [], rows: [], totalRows: 0 }
    return { columns: [], indexes: [], foreignKeys: [], options: {} }
  })
  const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(TableView) }) }), {
    global: { plugins: [pinia, i18n], stubs: { RowModal: true } },
  })
  await flushPromises()
  expect(api).toHaveBeenCalledWith('GET', '/api/databases/app/metadata?schema=public')
  route.params.db = 'Other DB'
  catalog.schema = 'Odd Schema'
  await flushPromises()
  expect(api).toHaveBeenCalledWith('GET', '/api/databases/Other%20DB/metadata?schema=Odd%20Schema')
  wrapper.unmount()
})

function openTable() {
 const pinia = createPinia(); setActivePinia(pinia)
 vi.mocked(api).mockImplementation(async (_method, path) => {
  if (/\/tables(?:\?|$)/.test(path)) return { tables: [] }
  if (path.includes('/create-sql')) return { sql: 'CREATE TABLE visible (id int)' }
  if (path.includes('/pk')) return { columns: ['id'] }
  if (path.includes('/rows')) return { columns: [], rows: [], totalRows: 0 }
  return { columns: [], indexes: [], foreignKeys: [], options: {} }
 })
 return mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(TableView) }) }), { global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true, StructureEditor: true } } })
}
it('shows catalog details on a direct table link without mounting the sidebar', async () => {
 const pinia = createPinia()
 const catalog = useCatalogStore(pinia)
 catalog.schemaAware = true
 catalog.schema = 'Odd Schema'
 vi.mocked(api).mockImplementation(async (_method, path) => {
  if (path === '/api/databases/app/tables?schema=Odd%20Schema') {
   return { tables: [{ name: 'items', comment: 'Inventory ledger', isView: false }] }
  }
  return { columns: [], rows: [], totalRows: 0 }
 })
 const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(TableView) }) }), {
  global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true } },
 })
 await flushPromises()
 expect(wrapper.get('header').text()).toContain('Inventory ledger')
 wrapper.unmount()
})
it('opens SQL from the URL and retains schema when changing sections', async () => {
 route.query = { view: 'sql', schema: 'Odd Schema' }
 const wrapper = openTable(); await flushPromises()
 expect(wrapper.get('pre code').text()).toContain('CREATE TABLE visible')
 await wrapper.get('[data-section=structure]').trigger('click'); await flushPromises()
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
it('restores route filters and browser back into the visible grid',async()=>{
 route.query={filters:JSON.stringify([{column:'name',op:'=',value:'first'}]),page:'3',pageSize:'25'}
 const wrapper=openTable(); await flushPromises()
 expect(wrapper.findComponent(DataGrid).props('filters')).toEqual([{column:'name',op:'=',value:'first'}])
 expect(api).toHaveBeenCalledWith('GET',expect.stringContaining('offset=50'))
 route.query={filters:JSON.stringify([{column:'name',op:'=',value:'back'}])}; await flushPromises()
 expect(wrapper.findComponent(DataGrid).props('filters')).toEqual([{column:'name',op:'=',value:'back'}]);wrapper.unmount()
})
it('restores linked columns in table order and exports exactly those columns with active filters and sort', async () => {
 const filters = [{ column: 'id', op: '>', value: '5' }]
 const sort = { column: 'id', desc: true }
 route.query = { schema: 'part one', filters: JSON.stringify(filters), sort: JSON.stringify(sort), columns: JSON.stringify(['label, %', 'gone', 'label, %']) }
 const pinia = createPinia()
 const catalog = useCatalogStore(pinia)
 catalog.schemaAware = true
 catalog.schema = 'part one'
 const columns = [{ name: 'id', type: 'int', nullable: false }, { name: 'label, %', type: 'text', nullable: true }]
 vi.mocked(api).mockImplementation(async (_method, path) => /\/tables(?:\?|$)/.test(path) ? { tables: [] } : path.includes('/pk') ? { columns: ['id'] } : { columns, rows: [[6, 'row']], totalRows: 1 })
 const exportFile = vi.spyOn(download, 'downloadExport').mockResolvedValue()
 const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(TableView) }) }), { global: { plugins: [pinia, i18n], stubs: { DataGrid: true, RowModal: true } } })
 try {
  await flushPromises()
  const grid = wrapper.findComponent(DataGrid)
  expect(grid.props('visibleColumns')).toEqual(['label, %'])
  grid.vm.$emit('export-selection')
  await flushPromises()
  expect(exportFile).toHaveBeenCalledWith('app', {
   format: 'csv', tables: ['items'],
   query: { columns: ['label, %'], filters: [{ column: 'id', operator: '>', value: '5' }], sorts: [sort] },
  }, 'part one')
  vi.mocked(api).mockClear()
  push.mockImplementation(async ({ query }) => { route.query = query })
  grid.vm.$emit('update:visible-columns', ['id'])
  await flushPromises()
  expect(route.query.columns).toBe('["id"]')
  expect(grid.props('visibleColumns')).toEqual(['id'])
  grid.vm.$emit('update:visible-columns', columns.map(column => column.name))
  await flushPromises()
  expect(route.query).toEqual({ schema: 'part one', filters: JSON.stringify(filters), sort: JSON.stringify(sort) })
  route.query = { ...route.query, columns: '["missing"]' }
  await flushPromises()
  expect(grid.props('visibleColumns')).toEqual([])
  grid.vm.$emit('export-selection')
  await flushPromises()
  expect(exportFile).toHaveBeenCalledTimes(1)
  expect(api).not.toHaveBeenCalled()
 } finally {
  exportFile.mockRestore()
  wrapper.unmount()
 }
})
it('waits for full defaults before opening insert',async()=>{
 const wrapper=openTable();await flushPromises()
 let finish!: (value:unknown)=>void
 vi.mocked(api).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 await wrapper.get('[data-testid=table-add-row]').trigger('click');await flushPromises()
 expect(wrapper.findComponent(RowModal).props('show')).toBe(false)
 finish({columns:[{name:'status',type:'text',nullable:false,default:{value:'active'}}],foreignKeys:[]});await flushPromises()
 expect(wrapper.findComponent(RowModal).props('show')).toBe(true)
 expect(wrapper.findComponent(RowModal).props('columns')[0]!.default!.value).toBe('active');wrapper.unmount()
})
it('keeps schema and encoded table context on quick links and gates partitions',async()=>{
 const wrapper=openTable();await flushPromises()
 const catalog=useCatalogStore();catalog.schemaAware=true;catalog.schema='Odd Schema';await flushPromises()
 useSessionStore().sessionInfo={caps:{partitions:false}} as never
 const exportButton=()=>wrapper.findAll('button').find(button=>button.text()==='Экспорт' || button.text()==='navigationWorkflows.export')!
 await exportButton().trigger('click');expect(push).toHaveBeenCalledWith('/db/app/export?table=items&schema=Odd+Schema')
 expect(wrapper.find('[data-testid=table-partitions]').exists()).toBe(false)
 useSessionStore().sessionInfo!.caps.partitions=true;await flushPromises()
 expect(wrapper.find('[data-testid=table-partitions]').exists()).toBe(true)
 await wrapper.get('[data-testid=table-partitions]').trigger('click')
 expect(push).toHaveBeenLastCalledWith('/db/app/table/items/partitions?schema=Odd%20Schema')
 wrapper.unmount()
})
it('does not issue a duplicate rows request when its route query update is acknowledged',async()=>{
 const wrapper=openTable();await flushPromises()
 const before=vi.mocked(api).mock.calls.filter(([,path])=>path.includes('/rows')).length
 push.mockImplementationOnce(async ({query})=>{route.query=query})
 wrapper.findComponent(DataGrid).vm.$emit('update:page',2);await flushPromises()
 const after=vi.mocked(api).mock.calls.filter(([,path])=>path.includes('/rows')).length
 expect(after-before).toBe(1);wrapper.unmount()
})
it('keeps delayed primary keys when route filters change during initial loading',async()=>{
 const pinia=createPinia();setActivePinia(pinia)
 let finishKeys!:(value:unknown)=>void
 vi.mocked(api).mockImplementation(async (_method,path)=>{
  if (/\/tables(?:\?|$)/.test(path)) return { tables: [] }
  if(path.includes('/pk'))return new Promise(resolve=>{finishKeys=resolve})
  if(path.includes('/rows'))return {columns:[],rows:[],totalRows:0}
  return {columns:[],foreignKeys:[]}
 })
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(TableView)})}),{global:{plugins:[pinia,i18n],stubs:{DataGrid:true,RowModal:true}}})
 await flushPromises()
 route.query={filters:JSON.stringify([{column:'name',op:'=',value:'changed'}])};await flushPromises()
 finishKeys({columns:['id']});await flushPromises()
 expect(wrapper.findComponent(DataGrid).props('pkColumns')).toEqual(['id'])
 wrapper.unmount()
})
it('reloads updated metadata and rows when returning to data after structure',async()=>{
 const wrapper=openTable();await flushPromises()
 route.query={view:'structure'};await flushPromises()
 vi.mocked(api).mockImplementation(async (_method,path)=>path.includes('/pk')?{columns:['new_id']}:path.includes('/rows')?{columns:[{name:'new_id',type:'int',nullable:false}],rows:[[2]],totalRows:1}:{columns:[]})
 route.query={view:'data'};await flushPromises()
 expect(wrapper.findComponent(DataGrid).props('columns')).toEqual([{name:'new_id',type:'int',nullable:false}])
 expect(wrapper.findComponent(DataGrid).props('pkColumns')).toEqual(['new_id'])
 wrapper.unmount()
})
it('does not open insert if leaving a dirty structure section was cancelled',async()=>{
 route.query={view:'structure'}
 const wrapper=openTable();await flushPromises();vi.mocked(api).mockClear()
 push.mockResolvedValueOnce({type:4})
 const add=wrapper.get('[data-testid=table-add-row]')
 await add.trigger('click');await flushPromises()
 expect(wrapper.findComponent(RowModal).props('show')).toBe(false)
 expect(api).not.toHaveBeenCalled();wrapper.unmount()
})
it('opens insert from the single table header action without changing navigation', async () => {
  route.query = { view: 'data' }
  const wrapper = openTable()
  await flushPromises()
  expect(wrapper.get('h1').text()).toBe('items')
  await wrapper.get('[data-testid=table-add-row]').trigger('click')
  await flushPromises()
  expect(wrapper.getComponent(RowModal).props('show')).toBe(true)
  expect(push).not.toHaveBeenCalled()
  wrapper.unmount()
})

it('rejects a bulk-edit event when reliable primary keys are unavailable', async () => {
 const wrapper = openTable(); await flushPromises()
 vi.mocked(api).mockImplementation(async (_method, path) => path.includes('/pk') ? { columns: [] } : { columns: [], rows: [], totalRows: 0 })
 route.params.t = 'keyless'; await flushPromises()
 vi.mocked(api).mockClear()
 wrapper.findComponent(DataGrid).vm.$emit('edit-rows', [{ name: 'duplicate' }])
 await flushPromises()
 expect(wrapper.findComponent(BulkEdit).props('show')).toBe(false)
 expect(api).not.toHaveBeenCalled()
 wrapper.unmount()
})

it('rejects incomplete bulk identities even when primary-key metadata is available', async () => {
 const wrapper = openTable(); await flushPromises(); vi.mocked(api).mockClear()
 wrapper.findComponent(DataGrid).vm.$emit('edit-rows', [{ name: 'wrong key' }])
 await flushPromises()
 expect(wrapper.findComponent(BulkEdit).props('show')).toBe(false)
 expect(api).not.toHaveBeenCalled()
 wrapper.unmount()
})

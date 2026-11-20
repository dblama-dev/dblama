import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { api } from '../api/client'
import i18n from '../i18n'
import { useCatalogStore } from '../stores/catalog'
import DbView from './DbView.vue'
import TableTransfer from '../components/TableTransfer.vue'
vi.mock('../api/client', () => ({ api: vi.fn() }))
const push = vi.hoisted(() => vi.fn())
vi.mock('vue-router', () => ({ useRoute: () => ({ params: {db:'app db'}, query:{schema:'Odd Schema'} }), useRouter: () => ({ push,replace:vi.fn() }) }))
it('offers the current selection to cross-database table transfer', async () => {
 const pinia=createPinia();setActivePinia(pinia)
 vi.mocked(api).mockResolvedValue({ tables:[{name:'items',rows:3,sizeBytes:100,engine:'InnoDB',comment:'',isView:false}] })
 const wrapper=mount(DbView,{global:{plugins:[pinia,i18n],stubs:{DatabaseActions:true,TableTransfer:true}}})
 await flushPromises()
 expect(wrapper.text()).toContain('items')
 await wrapper.get('input[aria-label=items]').setValue(true)
 expect(wrapper.findComponent(TableTransfer).props('tables')).toEqual(['items'])
 expect(wrapper.findComponent(TableTransfer).props('db')).toBe('app db')
 wrapper.unmount()
})
it('selects all visible base tables and confirms a bulk truncate',async()=>{
 const pinia=createPinia();setActivePinia(pinia)
 vi.mocked(api).mockResolvedValue({tables:[{name:'items',rows:3,sizeBytes:100,isView:false},{name:'other',rows:2,sizeBytes:30,isView:false},{name:'view',rows:0,sizeBytes:0,isView:true}]})
 const wrapper=mount(DbView,{global:{plugins:[pinia,i18n],stubs:{DatabaseActions:true,TableTransfer:true}}});await flushPromises()
 await wrapper.get('[data-select-all]').setValue(true)
 expect(wrapper.findComponent(TableTransfer).props('tables')).toEqual(['items','other'])
 expect(api).not.toHaveBeenCalledWith('POST',expect.anything(),expect.anything())
 const {NPopconfirm}=await import('naive-ui')
 wrapper.findAllComponents(NPopconfirm)[0].vm.$emit('positive-click');await flushPromises()
 expect(api).toHaveBeenCalledWith('POST','/api/databases/app%20db/tables',{action:'truncate',tables:['items','other']})
 wrapper.unmount()
})

it('shows database operations on demand and offers bulk operations only after selection', async () => {
 const pinia = createPinia()
 vi.mocked(api).mockResolvedValue({tables:[{name:'items',rows:3,sizeBytes:100,isView:false}]})
 const wrapper = mount(DbView, {attachTo:document.body, global:{plugins:[pinia,i18n], stubs:{TableTransfer:true}}})
 await flushPromises()
 const { NPopconfirm } = await import('naive-ui')
 expect(wrapper.findAllComponents(NPopconfirm)).toHaveLength(0)
 expect(wrapper.find('.database-actions').isVisible()).toBe(false)
 const toggle = wrapper.get('[data-database-actions]')
 await toggle.trigger('click')
 expect(toggle.attributes('aria-expanded')).toBe('true')
 expect(wrapper.get('.database-actions').isVisible()).toBe(true)
 await wrapper.get('input[aria-label=items]').setValue(true)
 expect(wrapper.findAllComponents(NPopconfirm)).toHaveLength(2)
 await wrapper.get('input[aria-label=items]').setValue(false)
 expect(wrapper.findAllComponents(NPopconfirm)).toHaveLength(0)
 wrapper.unmount()
})

it('distinguishes an empty database from a search with no matches and can clear the filter', async () => {
 const pinia = createPinia()
 vi.mocked(api).mockResolvedValueOnce({tables:[]})
 const wrapper = mount(DbView, {global:{plugins:[pinia,i18n],stubs:{DatabaseActions:true,TableTransfer:true}}})
 await flushPromises()
 expect(wrapper.find('[data-empty-database]').exists()).toBe(true)
 vi.mocked(api).mockResolvedValueOnce({tables:[{name:'items',rows:1,sizeBytes:8,isView:false}]})
 await wrapper.get('[data-refresh-tables]').trigger('click'); await flushPromises()
 await wrapper.get('.db-view__controls input').setValue('missing')
 expect(wrapper.find('[data-no-matches]').exists()).toBe(true)
 expect(wrapper.find('[data-empty-database]').exists()).toBe(false)
 await wrapper.get('[data-clear-filter]').trigger('click')
 expect(wrapper.find('input[aria-label=items]').exists()).toBe(true)
 wrapper.unmount()
})

it('opens the row insert route for a base table in the current schema', async () => {
 const pinia = createPinia()
 setActivePinia(pinia)
 useCatalogStore().$patch({schema:'Odd Schema', schemaAware:true})
 vi.mocked(api).mockResolvedValue({tables:[{name:'table / 1',rows:3,sizeBytes:100,isView:false},{name:'view',rows:0,sizeBytes:0,isView:true}]})
 const wrapper = mount(DbView,{global:{plugins:[pinia,i18n],stubs:{DatabaseActions:true,TableTransfer:true}}})
 await flushPromises()
 const insert = wrapper.findAll('[data-insert-row]')
 expect(insert).toHaveLength(1)
 await insert[0]!.trigger('click')
 expect(push).toHaveBeenCalledWith('/db/app%20db/table/table%20%2F%201?view=data&action=insert&schema=Odd%20Schema')
 wrapper.unmount()
})

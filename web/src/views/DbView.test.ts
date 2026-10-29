import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { api } from '../api/client'
import i18n from '../i18n'
import DbView from './DbView.vue'
import TableTransfer from '../components/TableTransfer.vue'
vi.mock('../api/client', () => ({ api: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: {db:'app db'}, query:{schema:'Odd Schema'} }), useRouter: () => ({ push:vi.fn(),replace:vi.fn() }) }))
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

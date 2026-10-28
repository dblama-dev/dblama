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
 await wrapper.get('input[type=checkbox]').setValue(true)
 expect(wrapper.findComponent(TableTransfer).props('tables')).toEqual(['items'])
 expect(wrapper.findComponent(TableTransfer).props('db')).toBe('app db')
 wrapper.unmount()
})

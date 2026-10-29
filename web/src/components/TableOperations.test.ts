import {flushPromises,mount} from '@vue/test-utils'
import {createPinia,setActivePinia} from 'pinia'
import {NPopconfirm} from 'naive-ui'
import {expect,it,vi} from 'vitest'
import i18n from '../i18n'
import {api} from '../api/client'
import {useCatalogStore} from '../stores/catalog'
import TableOperations from './TableOperations.vue'
vi.mock('../api/client',()=>({api:vi.fn()}))
it('requires contextual confirmation for scoped single-table truncate and drop',async()=>{
 const pinia=createPinia();setActivePinia(pinia)
 const catalog=useCatalogStore();catalog.schemaAware=true;catalog.schema='Odd Schema'
 vi.mocked(api).mockResolvedValue({tables:[]})
 const wrapper=mount(TableOperations,{props:{db:'a/b',table:'x %'},global:{plugins:[pinia,i18n]}})
 expect(api).not.toHaveBeenCalled()
 expect(wrapper.text()).toContain('x %')
 wrapper.findAllComponents(NPopconfirm)[0].vm.$emit('positive-click');await flushPromises()
 expect(api).toHaveBeenCalledWith('POST','/api/databases/a%2Fb/tables/x%20%25/truncate?schema=Odd%20Schema')
 wrapper.findAllComponents(NPopconfirm)[1].vm.$emit('positive-click');await flushPromises()
 expect(api).toHaveBeenCalledWith('DELETE','/api/databases/a%2Fb/tables/x%20%25?schema=Odd%20Schema')
 expect(wrapper.emitted('dropped')).toHaveLength(1)
})
it('never truncates a view and drops it through the existing view-aware table endpoint',async()=>{
 const pinia=createPinia();setActivePinia(pinia)
 const catalog=useCatalogStore();catalog.tables=[{name:'report',isView:true}] as never
 vi.mocked(api).mockClear();vi.mocked(api).mockResolvedValue({tables:[]})
 const wrapper=mount(TableOperations,{props:{db:'app',table:'report'},global:{plugins:[pinia,i18n]}})
 wrapper.findAllComponents(NPopconfirm)[0].vm.$emit('positive-click');await flushPromises()
 expect(api).not.toHaveBeenCalled()
 wrapper.findAllComponents(NPopconfirm)[1].vm.$emit('positive-click');await flushPromises()
 expect(api).toHaveBeenCalledWith('DELETE','/api/databases/app/tables/report?isView=true')
 wrapper.unmount()
})

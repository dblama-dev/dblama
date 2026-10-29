import {flushPromises,mount} from '@vue/test-utils'
import {createPinia} from 'pinia'
import {defineComponent,h} from 'vue'
import {NInput,NNotificationProvider} from 'naive-ui'
import {expect,it,vi} from 'vitest'
import i18n from '../../i18n'
import {api} from '../../api/client'
import IndexEditor from './IndexEditor.vue'
vi.mock('../../api/client',()=>({api:vi.fn(),ApiError:class extends Error{}}))
it('reports the exact submitted indexes when a newer edit is made during the request',async()=>{
 let finish!:()=>void
 vi.mocked(api).mockImplementation(()=>new Promise<void>(resolve=>{finish=resolve}))
 const wrapper=mount(defineComponent({setup:()=>()=>h(NNotificationProvider,null,{default:()=>h(IndexEditor,{db:'app',table:'items',indexes:[{name:'original',type:'INDEX',columns:[{name:'id'}]}],columnNames:['id']})})}),{global:{plugins:[createPinia(),i18n]}})
 const editor=wrapper.findComponent(IndexEditor)
 const name=editor.findComponent(NInput)
 name.vm.$emit('update:value','submitted_A');await flushPromises()
 await editor.get('[data-testid=index-save]').trigger('click')
 name.vm.$emit('update:value','unsaved_B');await flushPromises()
 finish();await flushPromises()
 expect(api).toHaveBeenCalledWith('POST','/api/databases/app/tables/items/indexes',[{name:'submitted_A',type:'INDEX',columns:[{name:'id'}]}])
 expect(editor.emitted('saved')).toEqual([[[{name:'submitted_A',type:'INDEX',columns:[{name:'id'}]}]]])
 expect(name.props('value')).toBe('unsaved_B')
 wrapper.unmount()
})

import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { UiButton, UiCheckbox, UiInput, UiNotificationProvider } from '../ui'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import i18n from '../i18n'
import BulkEdit from './BulkEdit.vue'
vi.mock('../api/client',()=>({api:vi.fn()}))
describe('BulkEdit',()=>{
 it('confirms captured identities, stops on first failure and emits saved after partial writes',async()=>{
  const mockedApi=vi.mocked(api);mockedApi.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('permission denied'))
  const keys=[{id:'9007199254740993'},{id:'2'},{id:'3'}]
  const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(BulkEdit,{show:true,rowsPath:'/rows',columns:[{name:'notes',type:'text',nullable:true}],rowKeys:keys})})}),{global:{plugins:[createPinia(),i18n],stubs:{teleport:true}}})
  wrapper.find('.bulk-edit__field').findComponent(UiCheckbox).vm.$emit('update:checked',true)
  wrapper.findComponent(UiInput).vm.$emit('update:value','changed')
  await flushPromises()
  await wrapper.findAllComponents(UiButton).find(button=>button.attributes('data-testid')==='bulk-review')?.trigger('click')
  await flushPromises()
  expect(mockedApi).not.toHaveBeenCalled()
  keys[0]!.id='modified after review'
  await wrapper.findAllComponents(UiButton).find(button=>button.attributes('data-testid')==='bulk-confirm')?.trigger('click')
  await flushPromises()
  expect(mockedApi.mock.calls).toEqual([['PATCH','/rows',{where:{id:'9007199254740993'},values:{notes:'changed'}}],['PATCH','/rows',{where:{id:'2'},values:{notes:'changed'}}]])
  expect(wrapper.text()).toContain('permission denied')
  expect(wrapper.find('.bulk-edit__progress').text()).toContain('1')
  expect(wrapper.findComponent(BulkEdit).emitted('saved')).toHaveLength(1)
  expect(wrapper.findComponent(BulkEdit).emitted('update:show')).toBeUndefined()
  wrapper.unmount()
 })
})

it('stops future updates on cancellation and reports only completed writes',async()=>{
 let complete!:()=>void
 const mockedApi=vi.mocked(api);mockedApi.mockReset();mockedApi.mockImplementationOnce(()=>new Promise<void>(resolve=>{complete=resolve}))
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(BulkEdit,{show:true,rowsPath:'/rows',columns:[{name:'notes',type:'text',nullable:true}],rowKeys:[{id:1},{id:2}]})})}),{global:{plugins:[createPinia(),i18n],stubs:{teleport:true}}})
 wrapper.find('.bulk-edit__field').findComponent(UiCheckbox).vm.$emit('update:checked',true)
 wrapper.findComponent(UiInput).vm.$emit('update:value','update')
 await flushPromises();await wrapper.find('[data-testid="bulk-review"]').trigger('click');await flushPromises()
 await wrapper.find('[data-testid="bulk-confirm"]').trigger('click');await flushPromises()
 await wrapper.findAllComponents(UiButton).find(button=>button.text().includes('Остановить'))?.trigger('click')
 complete();await flushPromises()
 expect(mockedApi).toHaveBeenCalledTimes(1)
 expect(wrapper.find('.bulk-edit__progress').text()).toContain('1')
 expect(wrapper.text()).toContain('Предыдущие изменения сохранены')
 expect(wrapper.findComponent(BulkEdit).emitted('saved')).toHaveLength(1)
 wrapper.unmount()
})

it('blocks empty NOT NULL numeric updates before confirmation', async()=>{
 const mockedApi=vi.mocked(api);mockedApi.mockReset()
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(BulkEdit,{show:true,rowsPath:'/rows',columns:[{name:'count',type:'int',nullable:false}],rowKeys:[{id:1}]})})}),{global:{plugins:[createPinia(),i18n],stubs:{teleport:true}}})
 wrapper.find('.bulk-edit__field').findComponent(UiCheckbox).vm.$emit('update:checked',true)
 await flushPromises();await wrapper.find('[data-testid="bulk-review"]').trigger('click');await flushPromises()
 expect(wrapper.find('[data-testid="bulk-confirm"]').exists()).toBe(false)
 expect(wrapper.find('[role="alert"]').text()).toContain('count')
 expect(mockedApi).not.toHaveBeenCalled()
 wrapper.unmount()
})

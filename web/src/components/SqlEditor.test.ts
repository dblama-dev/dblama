import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import SqlEditor from './SqlEditor.vue'

it('keeps typed SQL when asynchronous completion metadata changes', async () => {
  const wrapper = mount(SqlEditor,{props:{modelValue:'SELECT draft',dialect:'mysql',schema:{}}})
  await wrapper.setProps({schema:{customers:['id','name']}})
  expect(wrapper.get('.cm-content').text()).toBe('SELECT draft')
  await wrapper.setProps({modelValue:'SELECT restored'})
  expect(wrapper.get('.cm-content').text()).toBe('SELECT restored')
  wrapper.unmount()
})
it('runs selected SQL through Ctrl+Enter without mutating the document', async () => {
  const wrapper = mount(SqlEditor,{props:{modelValue:'SELECT 1;',dialect:'postgres'}})
  await wrapper.get('.cm-content').trigger('keydown',{key:'Enter',code:'Enter',ctrlKey:true})
  expect(wrapper.emitted('execute')).toEqual([[undefined]])
  expect(wrapper.get('.cm-content').text()).toBe('SELECT 1;')
  wrapper.unmount()
})

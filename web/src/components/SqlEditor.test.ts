import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { EditorView } from '@codemirror/view'
import SqlEditor from './SqlEditor.vue'

it('keeps typed SQL when asynchronous completion metadata changes', async () => {
  const wrapper = mount(SqlEditor,{props:{modelValue:'SELECT draft',dialect:'mysql',schema:{}}})
  await wrapper.setProps({schema:{customers:['id','name']}})
  expect(wrapper.get('.cm-content').text()).toBe('SELECT draft')
  await wrapper.setProps({modelValue:'SELECT restored'})
  expect(wrapper.get('.cm-content').text()).toBe('SELECT restored')
  wrapper.unmount()
})
it('runs the full document through Ctrl+Enter when there is no selection', async () => {
  const wrapper = mount(SqlEditor,{props:{modelValue:'SELECT 1;',dialect:'postgres'}})
  await wrapper.get('.cm-content').trigger('keydown',{key:'Enter',code:'Enter',ctrlKey:true})
  expect(wrapper.emitted('execute')).toEqual([[undefined]])
  expect(wrapper.get('.cm-content').text()).toBe('SELECT 1;')
  wrapper.unmount()
})

it('runs a real SQL selection through Ctrl+Enter without mutating the document', async () => {
  const document = 'SELECT 1;\nSELECT 2;'
  const wrapper = mount(SqlEditor,{props:{modelValue:document,dialect:'postgres'}})
  const editor = EditorView.findFromDOM(wrapper.get('.cm-editor').element as HTMLElement)!
  editor.dispatch({selection:{anchor:10,head:19}})
  await wrapper.get('.cm-content').trigger('keydown',{key:'Enter',code:'Enter',ctrlKey:true})
  expect(wrapper.emitted('execute')).toEqual([['SELECT 2;']])
  expect(editor.state.doc.toString()).toBe(document)
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  wrapper.unmount()
})

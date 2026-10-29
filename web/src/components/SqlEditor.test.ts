import { flushPromises, mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { EditorView } from '@codemirror/view'
import SqlEditor from './SqlEditor.vue'
import { defineComponent, h, ref } from 'vue'
import { darkTheme, NConfigProvider } from 'naive-ui'

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

function luminance(color: string): number {
  const channels = color.match(/[\d.]+/g)!.slice(0, 3).map(Number).map(value => {
    const component = value / 255
    return component <= 0.04045 ? component / 12.92 : ((component + 0.055) / 1.055) ** 2.4
  })
  return channels[0]! * 0.2126 + channels[1]! * 0.7152 + channels[2]! * 0.0722
}
function contrast(foreground: string, background: string): number {
  const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a)
  return (values[0]! + 0.05) / (values[1]! + 0.05)
}
it('keeps SQL, selection and readable syntax/gutters when the application theme changes', async () => {
 const dark = ref(false)
 const document = 'SELECT 1;\nSELECT 2; -- retained'
 const wrapper = mount(defineComponent({ setup: () => () => h(NConfigProvider, { theme: dark.value ? darkTheme : null }, { default: () => h(SqlEditor, { modelValue: document }) }) }), { attachTo: window.document.body })
 const editor = EditorView.findFromDOM(wrapper.get('.cm-editor').element as HTMLElement)!
 editor.dispatch({ selection: { anchor: 10, head: 19 } })
 for (const next of [true, false]) {
  dark.value = next; await flushPromises()
  expect(editor.state.facet(EditorView.darkTheme)).toBe(next)
  expect(EditorView.findFromDOM(wrapper.get('.cm-editor').element as HTMLElement)).toBe(editor)
  expect(editor.state.doc.toString()).toBe(document)
  expect(editor.state.selection.main.from).toBe(10)
  expect(editor.state.selection.main.to).toBe(19)
  const background = getComputedStyle(editor.dom).backgroundColor
  const gutter = getComputedStyle(wrapper.get('.cm-gutters').element)
  const keyword = wrapper.findAll('.cm-line span').find(span => span.text() === 'SELECT')!
  expect(contrast(getComputedStyle(keyword.element).color, background)).toBeGreaterThanOrEqual(4.5)
  expect(contrast(gutter.color, gutter.backgroundColor)).toBeGreaterThanOrEqual(4.5)
  expect(Math.abs(luminance(gutter.backgroundColor) - luminance(background))).toBeLessThan(0.1)
 }
 expect(wrapper.findComponent(SqlEditor).emitted('update:modelValue')).toBeUndefined()
 wrapper.unmount()
})

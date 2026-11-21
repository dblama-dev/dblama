import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { UiButton, UiPopconfirm } from '../ui'
import { beforeEach, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import TableOperations from './TableOperations.vue'
vi.mock('../api/client', () => ({ api: vi.fn() }))
beforeEach(() => { vi.mocked(api).mockReset() })
const info = (name: string, isView = false) => ({ name, isView, rows: 0, sizeBytes: 0, engine: '', comment: '' })
function open(db = 'a/b', table = 'x %', schema: string | null = 'Odd Schema') {
 const pinia = createPinia(); setActivePinia(pinia)
 const catalog = useCatalogStore(); catalog.schemaAware = schema !== null; catalog.schema = schema
 return mount(TableOperations, { props: { db, table }, global: { plugins: [pinia, i18n] } })
}
it('loads authoritative object kind without a sidebar, then confirms scoped truncate and drop', async () => {
 vi.mocked(api).mockResolvedValue({ tables: [info('x %')] })
 const wrapper = open()
 expect(wrapper.findAllComponents(UiButton).every(button => button.props('disabled'))).toBe(true)
 wrapper.findAllComponents(UiPopconfirm)[0]!.vm.$emit('positive-click'); await flushPromises()
 expect(api).toHaveBeenCalledWith('GET', '/api/databases/a%2Fb/tables?schema=Odd%20Schema')
 expect(vi.mocked(api).mock.calls.every(([method]) => method === 'GET')).toBe(true)
 wrapper.findAllComponents(UiPopconfirm)[0]!.vm.$emit('positive-click'); await flushPromises()
 expect(api).toHaveBeenCalledWith('POST', '/api/databases/a%2Fb/tables/x%20%25/truncate?schema=Odd%20Schema')
 wrapper.findAllComponents(UiPopconfirm)[1]!.vm.$emit('positive-click'); await flushPromises()
 expect(api).toHaveBeenCalledWith('DELETE', '/api/databases/a%2Fb/tables/x%20%25?schema=Odd%20Schema')
 expect(wrapper.emitted('dropped')).toHaveLength(1)
 wrapper.unmount()
})
it('resolves a direct-loaded view independently of an empty catalog', async () => {
 vi.mocked(api).mockResolvedValue({ tables: [info('report', true)] })
 const wrapper = open('app', 'report', null); await flushPromises()
 wrapper.findAllComponents(UiPopconfirm)[0]!.vm.$emit('positive-click'); await flushPromises()
 expect(vi.mocked(api).mock.calls.every(([method]) => method === 'GET')).toBe(true)
 wrapper.findAllComponents(UiPopconfirm)[1]!.vm.$emit('positive-click'); await flushPromises()
 expect(api).toHaveBeenCalledWith('DELETE', '/api/databases/app/tables/report?isView=true')
 wrapper.unmount()
})
it.each(['missing', 'failed'])('keeps destructive actions disabled when object kind is %s', async state => {
 if (state === 'failed') vi.mocked(api).mockRejectedValue(new Error('catalog unavailable'))
 else vi.mocked(api).mockResolvedValue({ tables: [] })
 const wrapper = open(); await flushPromises()
 expect(wrapper.findAllComponents(UiButton).every(button => button.props('disabled'))).toBe(true)
 for (const confirm of wrapper.findAllComponents(UiPopconfirm)) confirm.vm.$emit('positive-click')
 await flushPromises()
 expect(vi.mocked(api).mock.calls.every(([method]) => method === 'GET')).toBe(true)
 if (state === 'failed') expect(wrapper.text()).toContain('catalog unavailable')
 wrapper.unmount()
})
it('ignores old schema kind responses and late mutation completion after changing scope', async () => {
 let finishOld!: (value: unknown) => void
 let finishDrop!: (value: unknown) => void
 vi.mocked(api).mockImplementation(async (method, path) => {
  if (method === 'DELETE') return new Promise(resolve => { finishDrop = resolve })
  if (path.endsWith('schema=Odd%20Schema')) return new Promise(resolve => { finishOld = resolve })
  return { tables: [info('x %', true)] }
 })
 const wrapper = open()
 useCatalogStore().schema = 'new'; await flushPromises()
 finishOld({ tables: [info('x %')] }); await flushPromises()
 expect(wrapper.findAllComponents(UiButton)[0]!.props('disabled')).toBe(true)
 wrapper.findAllComponents(UiPopconfirm)[1]!.vm.$emit('positive-click'); await flushPromises()
 expect(api).toHaveBeenCalledWith('DELETE', '/api/databases/a%2Fb/tables/x%20%25?isView=true&schema=new')
 await wrapper.setProps({ db: 'next', table: 'next' }); await flushPromises()
 const before = vi.mocked(api).mock.calls.length
 finishDrop(undefined); await flushPromises()
 expect(wrapper.emitted('dropped')).toBeUndefined()
 expect(vi.mocked(api).mock.calls).toHaveLength(before)
 wrapper.unmount()
})

import { flushPromises, mount } from '@vue/test-utils'
import { UiInput, UiPopconfirm } from '../ui'
import { beforeEach, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { api } from '../api/client'
import BulkUpsert from './BulkUpsert.vue'

vi.mock('../api/client', () => ({ api: vi.fn() }))
beforeEach(() => vi.mocked(api).mockReset())

it('requires explicit confirmation before applying a bulk upsert', async () => {
  vi.mocked(api).mockResolvedValue(undefined)
  const wrapper = mount(BulkUpsert, { props: { show: true, path: '/api/databases/app/tables/t/upsert' }, global: { plugins: [i18n], stubs: { teleport:true } } })
  wrapper.findComponent(UiInput).vm.$emit('update:value', '[{"id":1,"name":"exact"}]')
  await flushPromises()
  await wrapper.get('form').trigger('submit')
  expect(api).not.toHaveBeenCalled()
  wrapper.findComponent(UiPopconfirm).vm.$emit('positive-click')
  await flushPromises()
  expect(api).toHaveBeenCalledWith('POST', '/api/databases/app/tables/t/upsert', { rows: [{ id: 1, name: 'exact' }] })
  wrapper.unmount()
})

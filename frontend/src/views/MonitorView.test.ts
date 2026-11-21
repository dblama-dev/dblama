import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MonitorView from './MonitorView.vue'
import { adminMessages } from '../i18n/admin'
function response(body: unknown, status = 200): Response { return { ok: status < 400, status, json: async () => body } as Response }
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })
describe('MonitorView', () => {
  it('loads processes, status, variables and kills only after confirmation', async () => {
    const fetcher = vi.fn(async (url: string) => response(url === '/api/processes' ? { processes: [{ id: 7, user: 'alice', host: 'localhost', database: 'app', command: 'Query', seconds: 2, state: 'active', query: 'SELECT 1' }] } : url === '/api/status' ? { status: [{ name: 'Uptime', value: '20' }] } : { variables: [{ name: 'max_connections', value: '100' }] }))
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mount(MonitorView, { global: { stubs: { teleport: true }, plugins: [createI18n({ legacy: false, locale: 'en', messages: adminMessages })] } }); await flushPromises()
    expect(wrapper.text()).toContain('SELECT 1'); expect(wrapper.text()).toContain('max_connections'); expect(wrapper.text()).toContain('Uptime')
    await wrapper.get('[data-test="kill"]').trigger('click'); await flushPromises()
    expect(fetcher).not.toHaveBeenCalledWith('/api/processes/7/kill', expect.anything())
    await wrapper.get('[data-test="confirm-kill"]').trigger('click'); await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/processes/7/kill', expect.objectContaining({ method: 'POST' }))
    fetcher.mockClear(); await wrapper.get('[data-test="refresh"]').trigger('click'); await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/processes', expect.anything())
    wrapper.unmount()
  })
  it('stops automatic refresh after unmounting', async () => {
    vi.useFakeTimers()
    const fetcher = vi.fn(async (url: string) => response(url === '/api/processes' ? { processes: [] } : url === '/api/status' ? { status: [] } : { variables: [] }))
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mount(MonitorView, { global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: adminMessages })] } }); await flushPromises()
    await wrapper.get('[data-test="auto-refresh"]').setValue(true)
    await vi.advanceTimersByTimeAsync(5000); expect(fetcher.mock.calls.length).toBeGreaterThan(3)
    wrapper.unmount(); const count = fetcher.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000); expect(fetcher.mock.calls).toHaveLength(count)
  })
})

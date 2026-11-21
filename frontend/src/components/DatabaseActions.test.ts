import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { UiButton, UiInput, UiSelect } from '../ui'
import { api } from '../api/client'
import DatabaseActions from './DatabaseActions.vue'
import { databaseMessages } from '../i18n/databases'

vi.mock('../api/client', () => ({ api: vi.fn() }))
const request = vi.mocked(api)
function mountActions(db?: string) { return mount(DatabaseActions, { props: { db }, global: { plugins: [createPinia(), createI18n({ legacy: false, locale: 'en', messages: databaseMessages })] } }) }
function button(w: ReturnType<typeof mountActions>, text: string) { const b = w.findAllComponents(UiButton).find(b => b.text() === text); if (!b) throw Error(`Missing button ${text}`); return b }
beforeEach(() => { request.mockReset(); request.mockImplementation(async (method, path) => { if (method === 'GET' && path === '/api/metadata') return { columnTypes: [], operators: [], collations: [{ name: 'utf8mb4_unicode_ci', database: true }] }; return undefined }) })
describe('database actions', () => {
  it('creates a database with the chosen collation and emits changed after success', async () => {
    const w = mountActions()
    await button(w, 'Create database').trigger('click'); await flushPromises()
    w.findComponent(UiInput).vm.$emit('update:value', 'new db')
    w.findComponent(UiSelect).vm.$emit('update:value', 'utf8mb4_unicode_ci')
    await flushPromises(); await w.find('form').trigger('submit'); await flushPromises()
    expect(request).toHaveBeenCalledWith('POST', '/api/databases', { name: 'new db', collation: 'utf8mb4_unicode_ci' })
    expect(w.emitted('changed')).toHaveLength(1)
  })
  it('requires the database name before deletion and keeps a failed form visible', async () => {
    const w = mountActions('important')
    await button(w, 'Drop database').trigger('click'); await flushPromises()
    expect(button(w, 'Confirm drop').props('disabled')).toBe(true)
    w.findComponent(UiInput).vm.$emit('update:value', 'important'); await flushPromises()
    request.mockRejectedValueOnce(new Error('permission denied'))
    await w.find('form').trigger('submit'); await flushPromises()
    expect(w.text()).toContain('permission denied'); expect(w.emitted('dropped')).toBeUndefined()
    await w.find('form').trigger('submit'); await flushPromises()
    expect(request).toHaveBeenCalledWith('DELETE', '/api/databases/important')
    expect(w.emitted('dropped')).toEqual([['important']])
  })
  it('renames to a distinct name and reports it for route updates', async () => {
    const w = mountActions('old')
    await button(w, 'Rename database').trigger('click'); await flushPromises()
    expect(button(w, 'Rename').props('disabled')).toBe(true)
    w.findComponent(UiInput).vm.$emit('update:value', 'new'); await flushPromises()
    await w.find('form').trigger('submit'); await flushPromises()
    expect(request).toHaveBeenCalledWith('PATCH', '/api/databases/old', { name: 'new' })
    expect(w.emitted('renamed')).toEqual([['new']])
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { foreignKeyMessages } from '../../i18n/foreignKeys'
import { useCatalogStore } from '../../stores/catalog'
import ForeignKeyEditor from './ForeignKeyEditor.vue'
const initial = [{ name: 'parent fk', columns: ['tenant', 'parent'], refSchema: 'public', refTable: 'parents', refColumns: ['tenant', 'id'], onDelete: 'CASCADE', onUpdate: 'NO ACTION' }]
beforeEach(() => { setActivePinia(createPinia()); const c = useCatalogStore(); c.schemaAware = true; c.schema = 'Odd Schema' })
afterEach(() => vi.unstubAllGlobals())
function mountEditor(table = 'child') { return mount(ForeignKeyEditor, { props: { db: 'my db', table, modelValue: initial, columnNames: ['tenant', 'parent', 'other'] }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: foreignKeyMessages })] } }) }
it('preserves composite mapping order and saves explicit referenced schema', async () => {
 const fetch = vi.fn(async () => ({ ok: true, status: 204, json: async () => ({}) })); vi.stubGlobal('fetch', fetch)
 const w = mountEditor(); await w.findAll('[data-local-column]')[1].setValue('other'); await w.get('[data-save-fks]').trigger('click'); await flushPromises()
 expect(fetch).toHaveBeenCalledWith('/api/databases/my%20db/tables/child/foreign-keys?schema=Odd%20Schema', expect.objectContaining({ method: 'POST', body: JSON.stringify([{ ...initial[0], columns: ['tenant', 'other'] }]) })); expect(w.emitted('saved')).toHaveLength(1)
})
it('edits create-table constraints locally and does not show a separate save button', async () => {
 const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); const w = mountEditor('_new'); await w.get('[data-add-mapping]').trigger('click'); await flushPromises()
 expect(w.find('[data-save-fks]').exists()).toBe(false); const events = w.emitted('update:modelValue')!; expect((events[events.length - 1][0] as { columns: string[] }[])[0].columns).toEqual(['tenant', 'parent', '']); expect(fetch).not.toHaveBeenCalled()
})
it('loads referenced columns using the referenced PostgreSQL schema', async () => {
 const fetch = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ columns: [{ name: 'tenant' }, { name: 'id' }] }) })); vi.stubGlobal('fetch', fetch); const w = mountEditor(); await w.get('[data-load-reference]').trigger('click'); await flushPromises()
 expect(fetch).toHaveBeenCalledWith('/api/databases/my%20db/tables/parents?schema=public', expect.objectContaining({ method: 'GET' })); expect(w.findAll('option').some(o => o.text() === 'id')).toBe(true)
})

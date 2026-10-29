import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
import { objectMessages } from '../i18n/objects'
import ObjectsView from './ObjectsView.vue'
import SqlEditor from '../components/SqlEditor.vue'
import { sqlWorkflowMessages } from '../i18n/sqlWorkflows'
const state = vi.hoisted(() => ({ kind: 'views' }))
vi.mock('vue-router', async importOriginal => ({ ...(await importOriginal<typeof import('vue-router')>()), useRoute: () => ({ params: { db: 'my db', get kind() { return state.kind } }, query: {} }) }))
function response(body: unknown, status = 200) { return { ok: status < 400, status, json: async () => body } as Response }
beforeEach(() => { setActivePinia(createPinia()); state.kind = 'views'; const s = useSessionStore(); s.sessionInfo = { database: 'my db', driver: { name: 'PostgreSQL', flavor: 'postgres', version: '17', user: 'root', host: 'localhost' }, caps: { views: true, triggers: true, routines: true, procedures: true, events: false, partitions: true, schemas: true, users: false, monitor: false, moveCopy: false, comments: true, upsert: true, descendingIndexes: true, sequences: true } }; const c = useCatalogStore(); c.schemaAware = true; c.schema = 'Odd Schema' })
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })
function view() { return mount(ObjectsView, { global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: {en:{...objectMessages.en,...sqlWorkflowMessages.en}} })] } }) }
it('opens server source and saves through the schema-qualified object endpoint', async () => {
 const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response({ objects: [{ name: 'active', kind: 'VIEW', source: 'CREATE VIEW active AS SELECT 1' }] }) : response({}, 204)); vi.stubGlobal('fetch', fetch)
 const w = view(); await flushPromises(); await w.get('[data-object="active"]').trigger('click')
 expect(w.get('.cm-content').text()).toBe('CREATE VIEW active AS SELECT 1')
 w.findComponent(SqlEditor).vm.$emit('update:modelValue','CREATE OR REPLACE VIEW active AS SELECT 2'); await flushPromises(); await w.get('[data-save]').trigger('click'); await flushPromises()
 expect(fetch).toHaveBeenCalledWith('/api/databases/my%20db/views/active?schema=Odd%20Schema', expect.objectContaining({ method: 'PUT', body: expect.stringContaining('SELECT 2') }))
})
it('does not fetch unsupported events and shows the reason', async () => { state.kind = 'events'; const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); const w = view(); await flushPromises(); expect(w.text()).toContain('not supported'); expect(fetch).not.toHaveBeenCalled() })
it('calls a selected procedure with JSON parameters and renders returned rows', async () => {
 state.kind = 'routines'; vi.stubGlobal('fetch', vi.fn(async (_url: string, opts: RequestInit) => opts.method === 'GET' ? response({ objects: [{ name: 'echo', kind: 'PROCEDURE', identity: 'IN x text', source: 'CREATE PROCEDURE echo(IN x text)' }] }) : response({ results: [{ columns: [{ name: 'value', type: 'text', nullable: true }], rows: [['hello']], seconds: 0 }] })))
 const w = view(); await flushPromises(); await w.get('[data-object="echo"]').trigger('click'); await w.get('[data-arguments]').setValue('["hello"]'); await w.get('[data-call]').trigger('click'); await flushPromises(); expect(w.text()).toContain('hello')
})
it('requires confirmation before dropping a partition', async () => {
 state.kind = 'partitions'; useCatalogStore().tables = [{ name: 'items', rows: 1, sizeBytes: 1, engine: '', comment: '', isView: false }]
 const fetch = vi.fn(async () => response({ partitions: [{ name: 'p0', method: 'RANGE', expression: 'id', description: '10', rows: 1 }] })); vi.stubGlobal('fetch', fetch)
 const w = view(); await flushPromises(); await w.get('[data-partition-drop="p0"]').trigger('click'); expect(w.find('[role="alertdialog"]').exists()).toBe(true); expect(fetch.mock.calls).toHaveLength(1)
})

it('deletes the object shown at confirmation even if selection changes', async () => {
 state.kind = 'triggers'
 const fetch = vi.fn(async (_url: string, opts: RequestInit) =>
  opts.method === 'GET' ? response({objects:[
   {name:'same',kind:'TRIGGER',table:'a',source:'CREATE TRIGGER same'},
   {name:'same',kind:'TRIGGER',table:'b',source:'CREATE TRIGGER same'},
  ]}) : response({},204))
 vi.stubGlobal('fetch',fetch)
 const w=view(); await flushPromises()
 await w.findAll('[data-object="same"]')[0]!.trigger('click')
 await w.get('[data-delete]').trigger('click')
 await w.findAll('[data-object="same"]')[1]!.trigger('click')
 await w.get('[data-confirm]').trigger('click')
 await flushPromises()
 const deletion=fetch.mock.calls.find(([,opts])=>opts.method==='DELETE')
 expect(deletion?.[0]).toContain('table=a')
 expect(deletion?.[0]).not.toContain('table=b')
 w.unmount()
})

it('keeps an edited object when switching objects is cancelled', async () => {
 vi.stubGlobal('fetch',vi.fn(async()=>response({objects:[
  {name:'one',kind:'VIEW',source:'CREATE VIEW one AS SELECT 1'},
  {name:'two',kind:'VIEW',source:'CREATE VIEW two AS SELECT 2'},
 ]})))
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 const w=view();await flushPromises();await w.get('[data-object="one"]').trigger('click')
 w.findComponent(SqlEditor).vm.$emit('update:modelValue','CREATE VIEW one AS SELECT 3'); await flushPromises()
 await w.get('[data-object="two"]').trigger('click')
 expect(confirm).toHaveBeenCalled()
 expect((w.get('[data-name]').element as HTMLInputElement).value).toBe('one')
 w.unmount()
})

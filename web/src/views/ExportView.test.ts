import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NAlert, NButton, NCheckbox, NNotificationProvider, NSelect } from 'naive-ui'

import type { TableInfo } from '../api/types'
import i18n from '../i18n'
import { transferWorkflowMessages } from '../i18n/transferWorkflows'
i18n.global.mergeLocaleMessage('en', transferWorkflowMessages.en)
i18n.global.mergeLocaleMessage('ru', transferWorkflowMessages.ru)
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
import ExportView from './ExportView.vue'

// The view reads the :db route param only.
const routeState = vi.hoisted(() => ({ db: 'mydb', table: '' }))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { db: routeState.db }, query: { table: routeState.table } }),
}))

const tables: TableInfo[] = [
  { name: 'users', rows: 3, sizeBytes: 4096, engine: 'InnoDB', comment: '', isView: false },
  { name: 'orders', rows: 5, sizeBytes: 8192, engine: 'InnoDB', comment: '', isView: false },
  { name: 'v_active', rows: 0, sizeBytes: 0, engine: '', comment: '', isView: true },
]

// jsonRes is a JSON Response double for the tables fetch.
function jsonRes(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

// exportRes doubles the streaming dump response; the client reads the
// Content-Disposition header and the blob only.
function exportRes(status = 200, filename = 'dump.sql'): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (name: string) => (name.toLowerCase() === 'content-disposition' ? `attachment; filename="${filename}"` : null),
    },
    blob: () => Promise.resolve(new Blob(['--dump--'])),
    json: () => Promise.resolve({}),
  } as unknown as Response
}

/** Routes the mocked fetch by URL: tables vs export. */
function mockFetch(exportResponse: Response): ReturnType<typeof vi.fn> {
  return vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.endsWith('/tables')) return jsonRes(200, { tables })
    if (url.endsWith('/export')) return exportResponse
    throw new Error(`unexpected url: ${url}`)
  })
}

// mountView mounts the view inside the notification provider the app wraps
// it with; useNotification throws without it.
function mountView(pinia: Pinia) {
  const host = defineComponent({
    setup: () => () => h(NNotificationProvider, null, { default: () => h(ExportView) }),
  })
  return mount(host, { global: { plugins: [pinia, i18n] } })
}

describe('ExportView', () => {
  let pinia: Pinia
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    routeState.db = 'mydb'
    routeState.table = ''
    pinia = createPinia()
    setActivePinia(pinia)
    const session = useSessionStore()
    session.sessionInfo = {
      driver: { name: 'MySQL', flavor: 'mysql', version: '8.0.36', user: 'root', host: 'localhost' },
      caps: {
        views: true,
        triggers: true,
        routines: true,
        procedures: true,
        events: false,
        partitions: false,
        sequences: false,
        schemas: false,
        users: false,
        monitor: false,
        moveCopy: false,
        comments: true,
        upsert: false,
        descendingIndexes: false,
      },
      database: 'mydb',
    }
    fetchMock = mockFetch(exportRes())
    vi.stubGlobal('fetch', fetchMock)
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    URL.createObjectURL = vi.fn(() => 'blob:mock')
    URL.revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    // Notifications render into a body portal that outlives one mount.
    document.body.innerHTML = ''
  })

  it('renders SQL options and views alongside base tables', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    expect(view.findComponent(NSelect).exists()).toBe(true)
    // Include views, DROP/IF NOT EXISTS, gzip and all three objects.
    const checkboxes = view.findAllComponents(NCheckbox)
    expect(checkboxes).toHaveLength(7)
    expect(view.text()).toContain('DROP IF EXISTS')
    expect(view.text()).toContain('IF NOT EXISTS')
    expect(view.text()).toContain('Сжимать (gzip)')
    // SQL includes views and their dependencies.
    expect(view.text()).toContain('users')
    expect(view.text()).toContain('orders')
    expect(view.text()).toContain('v_active')
  })

  it('clears selected tables when the active schema changes', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()
    const view = wrapper.findComponent(ExportView)
    view.findAllComponents(NCheckbox)[4]!.vm.$emit('update:checked', true)
    await flushPromises()
    expect(view.findAllComponents(NCheckbox)[4]!.props('checked')).toBe(true)
    const catalog = useCatalogStore()
    catalog.schemaAware = true
    catalog.schema = 'other'
    await flushPromises()
    expect(view.findAllComponents(NCheckbox)[4]!.props('checked')).toBe(false)
    expect(fetchMock).toHaveBeenCalledWith('/api/databases/mydb/tables?schema=other', expect.anything())
    wrapper.unmount()
  })

  it('hides the sql-only checkboxes for a delimited format', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    view.findComponent(NSelect).vm.$emit('update:value', 'csv')
    await nextTick()
    // gzip plus the two table checkboxes; the sql-only ones are gone.
    expect(view.findAllComponents(NCheckbox)).toHaveLength(3)
  })

  it('disables submit when more than one table is selected for a delimited format', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    view.findComponent(NSelect).vm.$emit('update:value', 'csv')
    await nextTick()
    // After the csv switch the order is gzip, users, orders.
    // Exactly one table is fine.
    view.findAllComponents(NCheckbox)[1].vm.$emit('update:checked', true)
    await nextTick()
    expect(view.findComponent(NButton).attributes('disabled')).toBeUndefined()

    // Two tables exceed the one-table limit of delimited dumps.
    view.findAllComponents(NCheckbox)[2].vm.$emit('update:checked', true)
    await nextTick()
    expect(view.findComponent(NButton).attributes('disabled')).toBeDefined()
    expect(view.text()).toContain('ровно одну таблицу')
  })

  it('submits the chosen format, tables and options to the export endpoint', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    view.findComponent(NSelect).vm.$emit('update:value', 'csv')
    await nextTick()
    view.findAllComponents(NCheckbox)[1].vm.$emit('update:checked', true) // users
    await nextTick()
    await view.find('[data-testid="export-submit"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/databases/mydb/export',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-Requested-With': 'XMLHttpRequest' }),
      }),
    )
    const body = JSON.parse(String(fetchMock.mock.calls[1][1].body))
    expect(body).toEqual({ format: 'csv', csvEncoding: 'escaped', tables: ['users'], includeViews: false, dropIfExists: false, ifNotExists: false, gzip: false })
    // The dump reaches the browser as a download and the UI confirms it.
    expect(URL.createObjectURL).toHaveBeenCalled()
    // Notifications render into a document.body portal.
    expect(document.body.textContent).toContain('Экспорт завершён')
  })

  it('sends the sql options when checked and an empty table list for the whole database', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    view.findAllComponents(NCheckbox)[1].vm.$emit('update:checked', true) // dropIfExists
    view.findAllComponents(NCheckbox)[3].vm.$emit('update:checked', true) // gzip
    await nextTick()
    await view.find('[data-testid="export-submit"]').trigger('click')
    await flushPromises()

    const body = JSON.parse(String(fetchMock.mock.calls[1][1].body))
    expect(body).toEqual({ format: 'sql', mode: 'all', tables: [], includeViews: true, dropIfExists: true, ifNotExists: false, gzip: true })
  })

  it('warns and disables submit when the selected db differs from the login db', async () => {
    routeState.db = 'otherdb'
    useSessionStore().sessionInfo!.driver.flavor = 'postgres'
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    expect(view.findComponent(NAlert).text()).toContain('текущей базы подключения')
    expect(view.findComponent(NButton).attributes('disabled')).toBeDefined()
  })

  it('preselects a route table and permits another MySQL database', async () => {
    routeState.table = 'orders'; routeState.db = 'otherdb'
    const wrapper = mountView(pinia); await flushPromises()
    const view = wrapper.findComponent(ExportView)
    expect(view.find('[data-testid="export-submit"]').attributes('disabled')).toBeUndefined()
    await view.find('[data-testid="export-submit"]').trigger('click'); await flushPromises()
    expect(JSON.parse(String(fetchMock.mock.calls[1][1].body)).tables).toEqual(['orders'])
  })

  it('reports a server error envelope as a notification', async () => {
    fetchMock = mockFetch(
      jsonRes(400, { error: { code: 'bad_request', message: 'csv/tsv export requires exactly one table' } }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ExportView)
    await view.find('[data-testid="export-submit"]').trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('csv/tsv export requires exactly one table')
    expect(wrapper.text()).not.toContain('Экспорт завершён')
  })
})

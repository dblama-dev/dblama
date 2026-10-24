import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NButton, NCheckbox, NInput, NNotificationProvider, NRadioGroup, NSelect } from 'naive-ui'

import type { TableInfo } from '../api/types'
import i18n from '../i18n'
import ImportView from './ImportView.vue'

const routeState = vi.hoisted(() => ({ db: 'mydb' }))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { db: routeState.db } }),
}))

const tables: TableInfo[] = [
  { name: 'users', rows: 3, sizeBytes: 4096, engine: 'InnoDB', comment: '', isView: false },
  { name: 'orders', rows: 5, sizeBytes: 8192, engine: 'InnoDB', comment: '', isView: false },
]

// jsonRes is a JSON Response double for the tables fetch.
function jsonRes(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

/** streamRes doubles the NDJSON import stream: every chunk is one read. */
function streamRes(chunks: string[]): Response {
  const encoder = new TextEncoder()
  let i = 0
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: () =>
          i < chunks.length
            ? Promise.resolve({ done: false, value: encoder.encode(chunks[i++]) })
            : Promise.resolve({ done: true, value: undefined }),
      }),
    },
  } as unknown as Response
}

/** Routes the mocked fetch by URL: tables vs import. */
function mockFetch(importResponse: Response): ReturnType<typeof vi.fn> {
  return vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.endsWith('/tables')) return jsonRes(200, { tables })
    if (url.endsWith('/import')) return importResponse
    throw new Error(`unexpected url: ${url}`)
  })
}

// mountView mounts the view inside the notification provider the app wraps
// it with; useNotification throws without it.
function mountView(pinia: Pinia) {
  const host = defineComponent({
    setup: () => () => h(NNotificationProvider, null, { default: () => h(ImportView) }),
  })
  return mount(host, { global: { plugins: [pinia, i18n] } })
}

/** Installs a fake File on the file input and fires the change event. */
async function chooseFile(view: ReturnType<typeof mount>, name: string, contents: string): Promise<void> {
  const input = view.find('input[type="file"]')
  const file = new File([contents], name, { type: 'text/plain' })
  Object.defineProperty(input.element, 'files', { value: [file] })
  await input.trigger('change')
}

describe('ImportView', () => {
  let pinia: Pinia
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    routeState.db = 'mydb'
    pinia = createPinia()
    setActivePinia(pinia)
    fetchMock = mockFetch(streamRes([]))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    // Notifications render into a body portal that outlives one mount.
    document.body.innerHTML = ''
  })

  it('disables submit until a file is chosen, and for csv until a table is picked', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ImportView)
    expect(view.findComponent(NButton).attributes('disabled')).toBeDefined()

    await chooseFile(view, 'dump.sql', 'CREATE TABLE t (id int);')
    expect(view.findComponent(NButton).attributes('disabled')).toBeUndefined()

    // csv without a table is invalid per the backend contract.
    view.findComponent(NRadioGroup).vm.$emit('update:value', 'csv')
    await flushPromises()
    expect(view.findComponent(NButton).attributes('disabled')).toBeDefined()

    view.findComponent(NSelect).vm.$emit('update:value', 'users')
    await flushPromises()
    expect(view.findComponent(NButton).attributes('disabled')).toBeUndefined()
  })

  it('posts multipart with the fields before the file part and the X-Requested-With header', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ImportView)
    view.findComponent(NRadioGroup).vm.$emit('update:value', 'csv')
    await flushPromises()
    view.findComponent(NSelect).vm.$emit('update:value', 'users')
    await flushPromises()
    // Tweak the csv options: separator ";" and skipHeader on.
    const inputs = view.findAllComponents(NInput)
    await inputs[0].find('input').setValue(';')
    view.findAllComponents(NCheckbox)[0].vm.$emit('update:checked', true)
    await flushPromises()
    await chooseFile(view, 'rows.csv', 'a;b\n1;2\n')

    await view.find('[data-testid="import-submit"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/databases/mydb/import',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-Requested-With': 'XMLHttpRequest' }),
      }),
    )
    const form = fetchMock.mock.calls[1][1].body as FormData
    // The server streams parts in order and keeps the file part open, so
    // every option field must precede the file.
    expect([...form.keys()]).toEqual(['type', 'table', 'separator', 'enclosure', 'skipHeader', 'file'])
    expect(form.get('type')).toBe('csv')
    expect(form.get('table')).toBe('users')
    expect(form.get('separator')).toBe(';')
    expect(form.get('enclosure')).toBe('')
    expect(form.get('skipHeader')).toBe('true')
    expect(form.get('file')).toBeInstanceOf(File)
  })

  it('updates progress from NDJSON lines and invalidates the catalog on final ok', async () => {
    // All three records arrive in a single chunk — the reader must split
    // the buffered lines itself.
    fetchMock = mockFetch(
      streamRes([
        '{"done":1,"total":4}\n{"done":4,"total":4}\n{"done":4,"total":4,"ok":true}\n',
      ]),
    )
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ImportView)
    await chooseFile(view, 'dump.sql', 'INSERT INTO t VALUES (1);')
    await view.find('[data-testid="import-submit"]').trigger('click')
    await flushPromises()

    // Progress reached done/total = 4/4 = 100%.
    expect(view.find('[data-testid="import-progress"]').text()).toContain('100')
    // Notifications render into a document.body portal.
    expect(document.body.textContent).toContain('Импорт завершён')
    // The catalog was refetched once after the import (two /tables calls
    // in total: the initial one on mount and the invalidate).
    const tableCalls = fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/tables'))
    expect(tableCalls).toHaveLength(2)
  })

  it('stops with an error notification on an error line and does not invalidate', async () => {
    fetchMock = mockFetch(
      streamRes(['{"done":1,"total":4}\n{"done":1,"total":4,"error":"boom near line 2"}\n']),
    )
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ImportView)
    await chooseFile(view, 'dump.sql', 'INSERT INTO t VALUES ();')
    await view.find('[data-testid="import-submit"]').trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('boom near line 2')
    expect(wrapper.text()).not.toContain('Импорт завершён')
    const tableCalls = fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/tables'))
    expect(tableCalls).toHaveLength(1)
  })

  it('shows the csv options only for the csv type', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(ImportView)
    expect(view.text()).not.toContain('Разделитель')

    view.findComponent(NRadioGroup).vm.$emit('update:value', 'csv')
    await flushPromises()
    expect(view.text()).toContain('Разделитель')
    expect(view.text()).toContain('Ограничитель')
    expect(view.text()).toContain('Пропустить строку заголовка')
  })
})

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { QueryResult } from '../api/types'
import i18n from '../i18n'
import ResultGrid from './ResultGrid.vue'
import { downloadBlob } from '../lib/download'

vi.mock('../lib/download', () => ({ downloadBlob: vi.fn() }))

const rowsResult: QueryResult = {
  columns: [
    { name: 'id', type: 'int', nullable: false },
    { name: 'name', type: 'varchar', nullable: true },
  ],
  rows: [
    [1, 'alice'],
    [2, null],
  ],
  seconds: 0.0125,
}

function mountGrid(result: QueryResult, index = 1) {
  return mount(ResultGrid, {
    props: { result, index },
    global: { plugins: [i18n] },
  })
}

describe('ResultGrid', () => {
  it('renders the statement number, column headers and cell values', () => {
    const wrapper = mountGrid(rowsResult, 3)

    expect(wrapper.text()).toContain('Результат 3')
    const headers = wrapper.findAll('th').map((th) => th.text())
    expect(headers).toEqual(['id', 'name'])
    const text = wrapper.text()
    expect(text).toContain('alice')
    expect(text).toContain('1')
  })

  it('renders null cells as an italic NULL placeholder', () => {
    const wrapper = mountGrid(rowsResult)

    const nulls = wrapper.findAll('.resultgrid__null')
    expect(nulls).toHaveLength(1)
    expect(nulls[0].text()).toBe('NULL')
    // Match the DataGrid NULL style: italic.
    expect(nulls[0].attributes('style')).toContain('font-style: italic')
    // The plain text "NULL" only appears through the styled placeholder.
    expect(wrapper.text()).toContain('NULL')
  })

  it('shows the affected-rows message when there are no rows', () => {
    const wrapper = mountGrid({
      columns: [],
      rows: [],
      affectedRows: 3,
      seconds: 0.2,
    })

    expect(wrapper.text()).toContain('Затронуто строк: 3')
    expect(wrapper.findAll('th')).toHaveLength(0)
  })

  it('renders a DDL outcome whose empty result slices serialize as null', () => {
    const wrapper = mountGrid({ columns: null, rows: null, seconds: 0.2 }, 4)

    expect(wrapper.find('.resultgrid__index').text()).toBe('Результат 4')
    expect(wrapper.find('.resultgrid__footer').text()).toBe('0.20 с')
    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('shows affected rows when the server sends null result slices', () => {
    const wrapper = mountGrid({ columns: null, rows: null, affectedRows: 2, seconds: 0.2 })

    expect(wrapper.find('.resultgrid__affected').text()).toBe('Затронуто строк: 2')
  })

  it('keeps column headers for an empty SELECT whose rows serialize as null', () => {
    const wrapper = mountGrid({ ...rowsResult, rows: null })

    expect(wrapper.findAll('th').map(header => header.text())).toEqual(['id', 'name'])
    expect(wrapper.findAll('tbody tr')).toHaveLength(0)
  })

  it('shows the truncated badge for a truncated result', () => {
    const wrapper = mountGrid({ ...rowsResult, truncated: true })

    expect(wrapper.text()).toContain('Обрезано')
  })

  it('downloads the displayed result by position and identifies a truncated result in the filename', async () => {
    const wrapper = mountGrid({
      columns: ['same', 'same', 'line,break', 'amount'].map(name => ({ name, type: 'text', nullable: true })),
      rows: [
        [null, '', 'first,"one"\nsecond', '9007199254740993.000000'],
        ['\\N', '\\path', 'русский', '0x00ff'],
      ],
      truncated: true, seconds: 0.1,
    }, 2)
    await wrapper.get('button').trigger('click')
    const [blob, filename] = vi.mocked(downloadBlob).mock.lastCall!
    expect(filename).toBe('query-result-2-truncated.csv')
    expect(blob.type).toBe('text/csv;charset=utf-8')
    expect(await blob.text()).toBe('same,same,"line,break",amount\n\\N,,"first,""one""\nsecond",9007199254740993.000000\n\\\\N,\\\\path,русский,0x00ff\n')
  })

  it('exports headers for an empty SELECT, preserves a single empty field, and omits the action for DDL', async () => {
    const wrapper = mountGrid({ columns: [{ name: 'v', type: 'text', nullable: false }], rows: null, seconds: 0 })
    await wrapper.get('button').trigger('click')
    const [headersOnly, name] = vi.mocked(downloadBlob).mock.lastCall!
    expect(name).toBe('query-result-1.csv')
    expect(await headersOnly.text()).toBe('v\n')
    await wrapper.setProps({ result: { ...wrapper.props('result'), rows: [['']] } })
    await wrapper.get('button').trigger('click')
    const [emptyField] = vi.mocked(downloadBlob).mock.lastCall!
    expect(await emptyField.text()).toBe('v\n""\n')
    await wrapper.setProps({ result: { columns: null, rows: null, affectedRows: 1, seconds: 0 } })
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('does not show the truncated badge for a complete result', () => {
    const wrapper = mountGrid(rowsResult)

    expect(wrapper.text()).not.toContain('Обрезано')
  })

  it('prints the seconds footer with two decimals', () => {
    const wrapper = mountGrid(rowsResult)

    expect(wrapper.find('.resultgrid__footer').text()).toBe('0.01 с')
  })
})

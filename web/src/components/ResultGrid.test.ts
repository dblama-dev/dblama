import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { QueryResult } from '../api/types'
import i18n from '../i18n'
import ResultGrid from './ResultGrid.vue'

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

    expect(wrapper.text()).toContain('#3')
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

  it('shows the truncated badge for a truncated result', () => {
    const wrapper = mountGrid({ ...rowsResult, truncated: true })

    expect(wrapper.text()).toContain('Обрезано')
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

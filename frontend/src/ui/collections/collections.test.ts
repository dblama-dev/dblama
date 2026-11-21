import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { UiDataTable, UiPagination, UiTree } from './index'

const global = { plugins: [i18n] }

describe('collection controls', () => {
  it('toggles the current page from a mixed selection without removing other page keys', async () => {
    const wrapper = mount(UiDataTable, {
      props: {
        columns: [{ type: 'selection' }, { key: 'name', title: 'Name' }],
        data: [{ name: 'a' }, { name: 'b' }],
        rowKey: row => String(Reflect.get(row, 'name')),
        checkedRowKeys: ['outside', 'b'],
      },
      global,
    })
    const all = wrapper.get<HTMLInputElement>('thead input')
    expect(all.element.indeterminate).toBe(true)
    await all.setValue(true)
    expect(wrapper.emitted('update:checked-row-keys')?.at(-1)).toEqual([['outside', 'b', 'a']])
    await wrapper.setProps({ checkedRowKeys: ['outside', 'a', 'b'] })
    expect(all.element.indeterminate).toBe(false)
    expect(all.element.checked).toBe(true)
    await all.setValue(false)
    expect(wrapper.emitted('update:checked-row-keys')?.at(-1)).toEqual([['outside']])
    wrapper.unmount()
  })

  it('keeps tree keyboard navigation and node actions independent', async () => {
    const star = vi.fn()
    const wrapper = mount(UiTree, {
      props: {
        data: [{ key: 'group', label: 'Tables', children: [
          { key: 'table:a', label: 'a', suffix: () => h('button', { type: 'button', onClick: star }, '★') },
          { key: 'table:b', label: 'b' },
        ] }],
        expandedKeys: [], selectedKeys: [],
      },
      attachTo: document.body, global,
    })
    try {
      const group = wrapper.get<HTMLElement>('[aria-label="Tables"]')
      group.element.focus()
      await group.trigger('keydown', { key: 'ArrowRight' })
      expect(wrapper.emitted('update:expanded-keys')).toEqual([[['group']]])
      await wrapper.setProps({ expandedKeys: ['group'] })
      await group.trigger('keydown', { key: 'ArrowRight' })
      const child = wrapper.get<HTMLElement>('[aria-label="a"]')
      expect(document.activeElement).toBe(child.element)
      await child.get('button').trigger('click')
      expect(star).toHaveBeenCalledOnce()
      expect(wrapper.emitted('update:selected-keys')).toBeUndefined()
      await child.trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('update:selected-keys')).toEqual([[['table:a']]])
      await child.trigger('keydown', { key: 'ArrowLeft' })
      expect(document.activeElement).toBe(group.element)
      await group.trigger('keydown', { key: 'ArrowLeft' })
      expect(wrapper.emitted('update:expanded-keys')?.at(-1)).toEqual([[]])
    } finally { wrapper.unmount() }
  })

  it('keeps the current page visible between endpoints, including in the compact pager', async () => {
    const wrapper = mount(UiPagination, { props: { page: 257, pageSize: 25, itemCount: 10000 }, global })
    expect(wrapper.get('[aria-current=page]').text()).toBe('257')
    expect(wrapper.findAll('.ui-pagination-item').at(-1)?.text()).toBe('400')
    await wrapper.setProps({ pageSlot: 3 })
    expect(wrapper.findAll('.ui-pagination-item').map(button => button.text())).toEqual(['256', '257', '258'])
    await wrapper.get('button:last-of-type').trigger('click')
    expect(wrapper.emitted('update:page')).toEqual([[258]])
    wrapper.unmount()
  })
})

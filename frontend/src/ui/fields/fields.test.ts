import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { UiCheckbox, UiSelect } from './index'

describe('form controls', () => {
  it('uses a native labelled checkbox and ignores disabled changes', async () => {
    const view = mount(UiCheckbox, { props: { label: 'Allow NULL', checked: false }, attachTo: document.body })
    const input = view.get('input[type=checkbox]').element as HTMLInputElement
    expect(input.labels?.[0]?.textContent).toBe('Allow NULL')
    input.labels![0]!.click()
    expect(view.emitted('update:checked')).toEqual([[true]])
    await view.setProps({ checked: true, disabled: true })
    input.labels![0]!.click()
    expect(view.emitted('update:checked')).toHaveLength(1)
    view.unmount()
  })

  it('selects an option via the keyboard using its original numeric value', async () => {
    const view = mount(UiSelect, { attrs: { 'aria-label': 'Choose a column' }, props: { value: null, filterable: true, options: [
      { label: 'First', value: 7 }, { label: 'Second', value: 9 },
    ] }, attachTo: document.body })
    const input = view.get('input[role=combobox]')
    await input.trigger('click')
    expect(document.querySelector('[role=listbox]')?.getAttribute('aria-label')).toBe('Choose a column')
    await input.setValue('Second')
    await input.trigger('keydown', { key: 'Enter' })
    expect(view.emitted('update:value')).toEqual([[9]])
    expect(input.attributes('aria-expanded')).toBe('false')
    view.unmount()
  })

  it('commits a manually typed foreign key before lookup completes and respects an explicit choice', async () => {
    const view = mount(UiSelect, { props: { value: 2, remote: true, filterable: true, tag: true, options: [
      { label: 'Alice (1)', value: 1 }, { label: 'Jane (2)', value: 2 },
    ] }, attachTo: document.body })
    const input = view.get('input[role=combobox]')
    await input.setValue('987654')
    await input.trigger('keydown', { key: 'Enter' })
    expect(view.emitted('update:value')).toEqual([['987654']])
    await input.setValue('Alice')
    await view.setProps({ options: [{ label: 'Jane (2)', value: 2 }, { label: 'Alice Finch (11)', value: 11 }, { label: 'Alice C. (12)', value: 12 }] })
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'Enter' })
    expect(view.emitted('update:value')?.at(-1)).toEqual([12])
    await input.setValue('11')
    await input.trigger('keydown', { key: 'Enter' })
    expect(view.emitted('update:value')?.at(-1)).toEqual([11])
    view.unmount()
  })

  it('retains multiple selections and keyboard Escape leaves the choices unchanged', async () => {
    const view = mount(UiSelect, { props: { value: ['a'], multiple: true, options: [
      { label: 'A', value: 'a' }, { label: 'B', value: 'b' },
    ] }, attachTo: document.body })
    const input = view.get('input[role=combobox]')
    await input.trigger('click')
    await flushPromises()
    const option = [...document.querySelectorAll<HTMLElement>('[role=option]')].find(el => el.textContent?.trim() === 'B')!
    option.click()
    expect(view.emitted('update:value')).toEqual([[['a','b']]])
    await input.trigger('keydown', { key: 'Escape' })
    expect(input.attributes('aria-expanded')).toBe('false')
    expect(view.emitted('update:value')).toHaveLength(1)
    view.unmount()
  })
})

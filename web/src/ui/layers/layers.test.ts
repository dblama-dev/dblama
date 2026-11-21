import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import i18n from '../../i18n'
import { UiDropdown, UiModal, UiNotificationProvider, UiPopconfirm, useNotification } from './index'

enableAutoUnmount(afterEach)
const global = { plugins: [i18n] }
const body = () => document.body

function press(target: Element, key: string) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
}

describe('workspace overlays', () => {
  it('cancel and accept on a stopped-click trigger in a modal, returning focus without closing the parent', async () => {
    const confirm = vi.fn(), visible = ref(false)
    const app = mount(defineComponent({ setup: () => () => [
      h('button', { id: 'open-row', onClick: () => { visible.value = true } }, 'Edit'),
      h(UiModal, { show: visible.value, title: 'Edit row', 'onUpdate:show': (value: boolean) => { visible.value = value } }, {
        default: () => h(UiPopconfirm, { onPositiveClick: confirm }, {
          trigger: () => h('button', { id: 'remove-row', onClick: (event: MouseEvent) => event.stopPropagation() }, 'Delete'),
          default: () => 'Delete this row?',
        }),
      }),
    ] }), { attachTo: body(), global })
    const opener = app.get('#open-row')
    ;(opener.element as HTMLElement).focus()
    await opener.trigger('click')
    await flushPromises()
    const dialog = body().querySelector<HTMLDialogElement>('dialog')!
    const trigger = dialog.querySelector<HTMLButtonElement>('#remove-row')!
    trigger.focus()
    trigger.click()
    await flushPromises()
    const question = dialog.querySelector<HTMLElement>('[role=alertdialog]')!
    expect(question.textContent).toContain('Delete this row?')
    const cancel = question.querySelector('button')!
    expect(document.activeElement).toBe(cancel)
    press(cancel, 'Escape')
    await flushPromises()
    expect(dialog.querySelector('[role=alertdialog]')).toBeNull()
    expect(confirm).not.toHaveBeenCalled()
    expect(document.activeElement).toBe(trigger)
    trigger.click()
    await flushPromises()
    dialog.querySelectorAll<HTMLButtonElement>('[role=alertdialog] button')[1]!.click()
    await flushPromises()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(dialog.open).toBe(true)
    press(dialog, 'Escape')
    await flushPromises()
    expect(body().querySelector('dialog')).toBeNull()
    expect(document.activeElement).toBe(opener.element)
  })

  it('keeps a busy modal open when Escape is pressed', async () => {
    const close = vi.fn()
    mount(UiModal, { props: { show: true, title: 'Import', closeOnEsc: false, closable: false, 'onUpdate:show': close }, global, attachTo: body() })
    await flushPromises()
    const dialog = body().querySelector('dialog')!
    press(dialog, 'Escape')
    expect(close).not.toHaveBeenCalled()
    expect(dialog.open).toBe(true)
    expect(dialog.querySelector('button')).toBeNull()
  })

  it('moves through enabled menu items and dismisses outside without selecting', async () => {
    const select = vi.fn()
    const host = mount(defineComponent({ setup: () => () => [
      h(UiDropdown, { options: [{ key: 'first', label: 'First' }, { key: 'disabled', label: 'Disabled', disabled: true }, { key: 'last', label: 'Last' }], onSelect: select }, {
        default: () => h('button', { id: 'actions' }, 'Actions'),
      }),
      h('button', { id: 'outside' }, 'Outside'),
    ] }), { attachTo: body(), global })
    const trigger = host.get('#actions').element
    press(trigger, 'ArrowDown')
    await flushPromises()
    expect(body().querySelector('[role=menu]')?.getAttribute('aria-labelledby')).toBe(trigger.id)
    expect(document.activeElement?.textContent).toBe('First')
    press(document.activeElement!, 'ArrowDown')
    expect(document.activeElement?.textContent).toBe('Last')
    ;(document.activeElement as HTMLElement).click()
    await flushPromises()
    expect(select).toHaveBeenCalledExactlyOnceWith('last')
    expect(document.activeElement).toBe(trigger)
    await host.get('#actions').trigger('click')
    await flushPromises()
    await host.get('#outside').trigger('pointerdown')
    await nextTick()
    expect(body().querySelector('[role=menu]')).toBeNull()
    expect(select).toHaveBeenCalledTimes(1)
  })

  it('keeps an error visible above an open modal and after its close until dismissed', async () => {
    const visible = ref(true)
    const Editor = defineComponent({ setup() {
      const notify = useNotification()
      return () => h(UiModal, { title: 'Query', show: visible.value }, {
        default: () => h('button', { id: 'notify', onClick: () => notify.error({ title: 'SQL', content: 'Access denied' }) }, 'Run'),
      })
    } })
    mount(UiNotificationProvider, { slots: { default: Editor }, attachTo: body(), global })
    await flushPromises()
    body().querySelector<HTMLButtonElement>('#notify')!.click()
    await flushPromises()
    expect(body().querySelector('dialog [role=alert]')?.textContent).toContain('SQLAccess denied')
    visible.value = false
    await flushPromises()
    expect(body().querySelector('[role=alert]')?.textContent).toContain('Access denied')
    body().querySelector<HTMLButtonElement>('[role=alert] button')!.click()
    await nextTick()
    expect(body().querySelector('[role=alert]')).toBeNull()
  })
})

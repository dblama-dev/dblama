import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { UiInput, UiNotificationProvider, UiSelect } from '../ui'

import type { DriverMeta } from '../api/types'
import i18n from '../i18n'
import { useSessionStore } from '../stores/session'
import LoginView from './LoginView.vue'

// LoginView navigates via router.push after a successful login.
const push = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

const drivers: DriverMeta[] = [
  { id: 'mysql', name: 'MySQL', defaultPort: '3306' },
  { id: 'postgres', name: 'PostgreSQL', defaultPort: '5432' },
]

// res builds a minimal Response double: the client only reads ok, status
// and json() from it.
function res(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

// mountView mounts LoginView inside the notification provider it has in
// the app (App.vue wraps the router view with one); useNotification
// throws without it.
function mountView(pinia: Pinia) {
  const host = defineComponent({
    setup: () => () => h(UiNotificationProvider, null, { default: () => h(LoginView) }),
  })
  return mount(host, { global: { plugins: [pinia, i18n] } })
}

describe('LoginView', () => {
  let pinia: Pinia

  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem(
      'dbm-last-login',
      JSON.stringify({ driver: 'postgres', host: 'db.local', user: 'alice' }),
    )
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(res(200, drivers)))
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders all fields and prefills the persisted last login (never the password)', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(LoginView)
    // СУБД select + host, port, user, password, database inputs.
    expect(view.findComponent(UiSelect).exists()).toBe(true)
    const inputs = view.findAllComponents(UiInput).map((field) => field.find('input'))
    expect(inputs).toHaveLength(5)
    // Persisted driver/host/user come back; password stays empty.
    expect(inputs[0].element.value).toBe('db.local')
    expect(inputs[2].element.value).toBe('alice')
    expect(inputs[3].element.value).toBe('')
    expect(view.find('button[type="submit"]').text()).toContain('Подключиться')
  })

  it('uses the selected driver defaultPort as the port placeholder', async () => {
    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(LoginView)
    const portInput = view.findAllComponents(UiInput)[1].find('input')

    await view.findComponent(UiSelect).vm.$emit('update:value', 'postgres')
    await nextTick()
    expect(portInput.attributes('placeholder')).toBe('5432')

    await view.findComponent(UiSelect).vm.$emit('update:value', 'mysql')
    await nextTick()
    expect(portInput.attributes('placeholder')).toBe('3306')
  })

  it('offers an optional PostgreSQL database field and can connect with it empty', async () => {
    const store = useSessionStore()
    store.login = vi.fn().mockResolvedValue(undefined)
    const wrapper = mountView(pinia)
    await flushPromises()
    const view = wrapper.findComponent(LoginView)
    expect(view.get('#login-db').attributes('placeholder')).toBe('необязательно')
    await view.get('#login-password').setValue('secret')
    await view.get('form').trigger('submit')
    expect(store.login).toHaveBeenCalledWith(expect.objectContaining({ driver: 'postgres', database: undefined }))
  })

  it('submits the form through sessionStore.login and navigates to /server', async () => {
    const store = useSessionStore()
    store.login = vi.fn().mockResolvedValue(undefined)

    const wrapper = mountView(pinia)
    await flushPromises()

    const view = wrapper.findComponent(LoginView)
    const inputs = view.findAllComponents(UiInput).map((field) => field.find('input'))
    await inputs[0].setValue('127.0.0.1') // host
    await inputs[1].setValue('13306') // port
    await inputs[2].setValue('root') // user
    await inputs[3].setValue('secret') // password
    await inputs[4].setValue('mydb') // database
    await view.findComponent(UiSelect).vm.$emit('update:value', 'mysql')
    await nextTick()

    await view.find('form').trigger('submit')

    expect(store.login).toHaveBeenCalledWith({
      driver: 'mysql',
      host: '127.0.0.1',
      port: '13306',
      user: 'root',
      password: 'secret',
      database: 'mydb',
    })
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/server')
    // driver+host+user persist, the password never does.
    expect(localStorage.getItem('dbm-last-login')).toBe(
      JSON.stringify({ driver: 'mysql', host: '127.0.0.1', user: 'root' }),
    )
  })
})

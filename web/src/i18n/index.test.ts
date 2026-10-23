import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// Each test re-imports the i18n module so the singleton is rebuilt with
// the localStorage state the test staged.
async function freshI18n() {
  vi.resetModules()
  return (await import('./index')).default
}

describe('i18n locale persistence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('defaults to ru when nothing is persisted', async () => {
    const i18n = await freshI18n()
    expect(i18n.global.locale.value).toBe('ru')
  })

  it('starts from the persisted dbm-locale', async () => {
    localStorage.setItem('dbm-locale', 'en')
    const i18n = await freshI18n()
    expect(i18n.global.locale.value).toBe('en')
  })

  it('persists locale changes back to localStorage', async () => {
    const i18n = await freshI18n()
    i18n.global.locale.value = 'en'
    // The persistence watcher flushes with the default pre queue.
    await nextTick()
    expect(localStorage.getItem('dbm-locale')).toBe('en')
  })
})

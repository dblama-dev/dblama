import { ref, type InjectionKey, type Ref } from 'vue'

const STORAGE_KEY = 'dbm-theme'

/** Injection key for the app-wide theme toggle provided by App.vue and
 * consumed by the topbar. */
export const themeToggleKey: InjectionKey<() => void> = Symbol('dbm-theme-toggle')

/**
 * setupTheme returns the app-wide dark mode flag persisted in localStorage
 * under the `dbm-theme` key ("dark" or "light").
 */
export function setupTheme(): { dark: Ref<boolean>; toggle: () => void } {
  const dark = ref(localStorage.getItem(STORAGE_KEY) === 'dark')

  function toggle(): void {
    dark.value = !dark.value
    localStorage.setItem(STORAGE_KEY, dark.value ? 'dark' : 'light')
  }

  return { dark, toggle }
}

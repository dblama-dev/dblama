import { ref, watch, type InjectionKey, type Ref } from 'vue'

export const themeModeKey: InjectionKey<Ref<boolean>> = Symbol('dblama-theme-mode')

const STORAGE_KEY = 'dblama-theme'

/**
 * setupTheme returns the app-wide dark mode flag persisted in localStorage
 * under the `dblama-theme` key ("dark" or "light").
 */
export function setupTheme(): Ref<boolean> {
  const dark = ref(localStorage.getItem(STORAGE_KEY) === 'dark')
  watch(dark, value => localStorage.setItem(STORAGE_KEY, value ? 'dark' : 'light'))
  return dark
}

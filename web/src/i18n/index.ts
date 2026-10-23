import { watch } from 'vue'
import { createI18n } from 'vue-i18n'

import ru from './locales/ru'
import en from './locales/en'

// The chosen locale round-trips through localStorage under `dbm-locale`:
// it is read here at setup and written back by the watcher below whenever
// the app switches locale.
const LOCALE_KEY = 'dbm-locale'

const stored = localStorage.getItem(LOCALE_KEY)
const i18n = createI18n({
  legacy: false,
  locale: stored === 'en' ? 'en' : 'ru',
  fallbackLocale: 'en',
  messages: { ru, en },
})

watch(
  i18n.global.locale,
  (locale) => {
    localStorage.setItem(LOCALE_KEY, locale)
  },
)

export default i18n

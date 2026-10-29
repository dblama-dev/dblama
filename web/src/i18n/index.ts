import { searchWorkflowMessages } from './searchWorkflows'
import { sqlWorkflowMessages } from './sqlWorkflows'
import { rowWorkflowMessages } from './rowWorkflows'
import { navigationWorkflowMessages } from './navigationWorkflows'
import { transferWorkflowMessages } from './transferWorkflows'
import { watch } from 'vue'
import { createI18n } from 'vue-i18n'

import { adminMessages } from './admin'
import { objectMessages } from './objects'
import { foreignKeyMessages } from './foreignKeys'
import { databaseMessages } from './databases'
import ru from './locales/ru'
import en from './locales/en'

// The chosen locale round-trips through localStorage under `dbm-locale`:
// it is read here at setup and written back by the watcher below whenever
// the app switches locale.
const LOCALE_KEY = 'dbm-locale'

let stored: string | null = null
try { stored = localStorage.getItem(LOCALE_KEY) } catch { /* Browser storage may be unavailable. */ }
const i18n = createI18n({
  legacy: false,
  locale: stored === 'en' ? 'en' : 'ru',
  fallbackLocale: 'en',
  messages: { ru: { ...ru, ...adminMessages.ru, ...objectMessages.ru, ...databaseMessages.ru, ...foreignKeyMessages.ru, ...sqlWorkflowMessages.ru, ...rowWorkflowMessages.ru, ...navigationWorkflowMessages.ru, ...transferWorkflowMessages.ru, ...searchWorkflowMessages.ru }, en: { ...en, ...adminMessages.en, ...objectMessages.en, ...databaseMessages.en, ...foreignKeyMessages.en, ...sqlWorkflowMessages.en, ...rowWorkflowMessages.en, ...navigationWorkflowMessages.en, ...transferWorkflowMessages.en, ...searchWorkflowMessages.en } },
})

watch(
  i18n.global.locale,
  (locale) => {
    try { localStorage.setItem(LOCALE_KEY, locale) } catch { /* Locale still works for this session. */ }
  },
)

export default i18n

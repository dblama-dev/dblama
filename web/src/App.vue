<template>
  <n-config-provider :theme="dark ? darkTheme : null" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-notification-provider>
      <router-view />
    </n-notification-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, provide } from 'vue'
import {
  darkTheme,
  dateEnUS,
  dateRuRU,
  enUS,
  NConfigProvider,
  NNotificationProvider,
  ruRU,
} from 'naive-ui'

import i18n from './i18n'
import { setupTheme, themeToggleKey } from './theme'

const { dark, toggle } = setupTheme()
provide(themeToggleKey, toggle)

// Keep naive-ui's built-in texts (dates, empty states, pagination) in the
// same language as the app i18n.
const naiveLocale = computed(() => (i18n.global.locale.value === 'en' ? enUS : ruRU))
const naiveDateLocale = computed(() => (i18n.global.locale.value === 'en' ? dateEnUS : dateRuRU))
</script>

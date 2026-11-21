<template>
  <UiNotificationProvider><router-view /></UiNotificationProvider>
</template>
<script setup lang="ts">
import { provide, watchEffect } from 'vue'
import { UiNotificationProvider } from './ui/layers'
import i18n from './i18n'
import { setupTheme, themeModeKey, themeToggleKey } from './theme'

const { dark, toggle } = setupTheme()
provide(themeModeKey, dark)
provide(themeToggleKey, toggle)
watchEffect(() => {
  document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'
  document.documentElement.lang = i18n.global.locale.value
})
</script>

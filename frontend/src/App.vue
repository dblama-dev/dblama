<template>
  <UiNotificationProvider><router-view /></UiNotificationProvider>
</template>
<script setup lang="ts">
import { provide, watchEffect } from 'vue'
import { UiNotificationProvider } from './ui/layers'
import i18n from './i18n'
import { setupTheme, themeModeKey } from './theme'

const dark = setupTheme()
provide(themeModeKey, dark)
watchEffect(() => {
  document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'
  document.documentElement.lang = i18n.global.locale.value
})
</script>

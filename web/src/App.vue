<template>
  <n-config-provider :theme="dark ? darkTheme : null">
    <n-notification-provider>
      <router-view />
    </n-notification-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { provide } from 'vue'
import { darkTheme, NConfigProvider, NNotificationProvider } from 'naive-ui'

import { setupTheme, themeToggleKey } from './theme'

const { dark, toggle } = setupTheme()
provide(themeToggleKey, toggle)
</script>

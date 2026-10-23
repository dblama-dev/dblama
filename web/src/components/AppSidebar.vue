<template>
  <aside class="app-sidebar">
    {{ t('sidebar.placeholder') }}
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<style scoped>
/* Empty placeholder: the object tree lands in П2. */
.app-sidebar {
  padding: 12px;
  font-size: 12px;
  opacity: 0.6;
}
</style>

<template>
  <section class="db-view">
    <n-empty :description="t('tree.selectTable')" />
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { NEmpty } from 'naive-ui'

const { t } = useI18n()
</script>

<style scoped>
.db-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}
</style>

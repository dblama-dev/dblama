<template>
  <section class="table-view">
    <n-breadcrumb>
      <n-breadcrumb-item>{{ db }}</n-breadcrumb-item>
      <n-breadcrumb-item>{{ table }}</n-breadcrumb-item>
    </n-breadcrumb>
    <n-tabs type="line" :value="view" @update:value="switchView">
      <n-tab name="data">{{ t('table.data') }}</n-tab>
      <n-tab name="structure">{{ t('table.structure') }}</n-tab>
    </n-tabs>
    <p class="table-view__stub">
      {{ view }}: {{ db }}.{{ table }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NBreadcrumb, NBreadcrumbItem, NTab, NTabs } from 'naive-ui'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

// Minimal stub for Tasks 12/14: real data and structure panes land later.
const db = computed(() => String(route.params.db ?? ''))
const table = computed(() => String(route.params.t ?? ''))
const view = computed(() => (route.query.view === 'structure' ? 'structure' : 'data'))

function switchView(next: string | number): void {
  void router.push({ query: { ...route.query, view: next } })
}
</script>

<style scoped>
.table-view {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.table-view__stub {
  opacity: 0.6;
}
</style>

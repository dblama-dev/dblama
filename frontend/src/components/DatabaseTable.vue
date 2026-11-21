<template>
  <div class="database-table db-panel">
    <div class="database-table__toolbar">
      <label class="db-search"><UiIcon name="search" /><input v-model="filter" type="search" :aria-label="t('server.filterDatabases')" :placeholder="t('server.filterDatabases')" /></label>
      <ui-button size="small" :loading="loading" @click="emit('refresh')">
        <UiIcon name="refresh" :size="14" />{{ t('server.refresh') }}
      </ui-button>
    </div>
    <ui-data-table
      :columns="columns"
      :data="visibleDatabases"
      :loading="loading"
      :row-props="rowProps"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiButton, UiDataTable, type DataTableColumns } from '../ui'

import type { DatabaseInfo } from '../api/types'
import UiIcon from './UiIcon.vue'
import { formatBytes } from '../lib/format'

const props = defineProps<{
  databases: DatabaseInfo[]
  loading?: boolean
}>()

const emit = defineEmits<{ refresh: []; open: [name: string] }>()

const { t } = useI18n()
const filter = ref('')
const visibleDatabases = computed(() => {
  const text = filter.value.trim().toLowerCase()
  return props.databases.filter(database => database.name.toLowerCase().includes(text))
})

const columns = computed<DataTableColumns<DatabaseInfo>>(() => [
  {
    title: t('server.columns.name'), key: 'name',
    render: row => h('div', [
      h('a', {
        href: '/db/' + encodeURIComponent(row.name),
        class: 'database-name',
        onClick(event: MouseEvent) {
          event.stopPropagation()
          if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
          event.preventDefault()
          emit('open', row.name)
        },
      }, [
        h('span', { class: 'db-object-icon' }, [h(UiIcon, { name: 'db', size: 15 })]),
        h('span', row.name),
      ]),
      row.collation && h('small', { class: 'database-collation' }, [
        h('span', { class: 'collation-label' }, t('server.columns.collation') + ' '), row.collation,
      ]),
    ]),
  },
  {
    title: t('server.columns.size'),
    key: 'sizeBytes',
    render: (row) => formatBytes(row.sizeBytes),
  },
  { title: t('server.columns.collation'), key: 'collation' },
])

// Clicking a row opens the database in the tree view.
const rowProps = (row: DatabaseInfo): Record<string, unknown> => ({
  style: 'cursor: pointer',
  onClick: () => emit('open', row.name),
})
</script>

<style scoped>
.database-table { overflow: hidden; min-width: 0; container-type: inline-size; }
.database-table__toolbar { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 12px 14px; border-bottom: 1px solid var(--db-line); }
.database-table :deep(.database-name) { display: inline-flex; align-items: center; gap: 10px; color: var(--db-ink); text-decoration: none; font: 550 13px var(--db-mono); overflow-wrap: anywhere; }
.database-table :deep(.database-name:hover) { color: var(--db-accent); text-decoration: underline; }
.database-table :deep(.database-collation) { display: none; }
@container (max-width: 480px) {
  .database-table :deep(:is(th, td, col):last-child) { display: none; }
  .database-table :deep(:is(th, td):nth-child(2)) { white-space: nowrap; text-align: end; }
  .database-table :deep(.database-collation) { display: block; margin-inline-start: 38px; overflow-wrap: anywhere; color: var(--db-muted); font: 11px var(--db-mono); }
  .database-table :deep(.collation-label) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
}
</style>

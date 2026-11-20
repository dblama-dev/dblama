<template>
  <div class="database-table">
    <div class="database-table__toolbar">
      <input v-model="filter" type="search" :aria-label="t('server.filterDatabases')" :placeholder="t('server.filterDatabases')" />
      <n-button size="small" :loading="loading" @click="emit('refresh')">
        {{ t('server.refresh') }}
      </n-button>
    </div>
    <n-data-table
      :columns="columns"
      :data="visibleDatabases"
      :loading="loading"
      :row-props="rowProps"
      :bordered="false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NDataTable, useThemeVars, type DataTableColumns } from 'naive-ui'

import type { DatabaseInfo } from '../api/types'
import { formatBytes } from '../lib/format'

const props = defineProps<{
  databases: DatabaseInfo[]
  loading?: boolean
}>()

const emit = defineEmits<{ refresh: []; open: [name: string] }>()

const { t } = useI18n()
const theme = useThemeVars()
const filter = ref('')
const visibleDatabases = computed(() => {
  const text = filter.value.trim().toLowerCase()
  return props.databases.filter(database => database.name.toLowerCase().includes(text))
})

const columns = computed<DataTableColumns<DatabaseInfo>>(() => [
  {
    title: t('server.columns.name'), key: 'name',
    render: row => h('a', {
      href: '/db/' + encodeURIComponent(row.name),
      style: { color: theme.value.primaryColorPressed, textUnderlineOffset: '3px' },
      onClick(event: MouseEvent) {
        event.stopPropagation()
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
        event.preventDefault()
        emit('open', row.name)
      },
    }, row.name),
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
.database-table__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.database-table__toolbar input { box-sizing: border-box; min-width: 0; width: min(310px, 100%); padding: 6px 10px; font: inherit; color: inherit; background: v-bind('theme.inputColor'); border: 1px solid v-bind('theme.borderColor'); border-radius: 4px; }
</style>

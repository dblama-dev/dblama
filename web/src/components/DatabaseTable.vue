<template>
  <div class="database-table">
    <div class="database-table__toolbar">
      <n-button size="small" :loading="loading" @click="emit('refresh')">
        {{ t('server.refresh') }}
      </n-button>
    </div>
    <n-data-table
      :columns="columns"
      :data="databases"
      :loading="loading"
      :bordered="false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NDataTable, type DataTableColumns } from 'naive-ui'

import type { DatabaseInfo } from '../api/types'
import { formatBytes } from '../lib/format'

defineProps<{
  databases: DatabaseInfo[]
  loading?: boolean
}>()

const emit = defineEmits<{ refresh: [] }>()

const { t } = useI18n()

const columns = computed<DataTableColumns<DatabaseInfo>>(() => [
  { title: t('server.columns.name'), key: 'name' },
  {
    title: t('server.columns.size'),
    key: 'sizeBytes',
    render: (row) => formatBytes(row.sizeBytes),
  },
  { title: t('server.columns.collation'), key: 'collation' },
])
</script>

<style scoped>
.database-table__toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
}
</style>

<template>
  <section class="export-view">
    <h2>{{ t('export.title') }}</h2>

    <n-alert v-if="dbMismatch" type="warning" data-testid="export-db-mismatch">
      {{ t('export.dbMismatch') }}
    </n-alert>

    <p>{{ t('transferWorkflows.context', { db, table: selected.join(', ') || t('transferWorkflows.allTables') }) }}</p>
    <div class="export-view__form">
      <label class="export-view__field">
        <span>{{ t('export.format') }}</span>
        <n-select
          v-model:value="format"
          :options="formatOptions"
          data-testid="export-format"
        />
      </label>

      <label v-if="isSql" class="export-view__field">
        <span>{{ t('transferWorkflows.mode') }}</span>
        <n-select v-model:value="mode" :options="modeOptions" data-testid="export-mode" />
      </label>
      <p v-if="isSql && mode === 'data' && catalog.schemaAware">{{ t('transferWorkflows.dataDependencies') }}</p>
      <n-alert v-if="!isSql" type="info">{{ t('transferWorkflows.csvExportEncoding') }}</n-alert>
      <div v-if="isSql && mode !== 'data'" class="export-view__field">
        <n-checkbox v-model:checked="includeViews">{{ t('export.includeViews') }}</n-checkbox>
        <n-checkbox v-model:checked="dropIfExists">{{ t('export.dropIfExists') }}</n-checkbox>
        <n-checkbox v-model:checked="ifNotExists">{{ t('export.ifNotExists') }}</n-checkbox>
      </div>
      <div class="export-view__field">
        <n-checkbox v-model:checked="gzip">{{ t('export.gzip') }}</n-checkbox>
      </div>

      <div class="export-view__field">
        <span>{{ t('export.tables') }}</span>
        <span v-if="isSql" class="export-view__hint">{{ t('export.tablesHint') }}</span>
        <span v-else-if="selected.length !== 1" class="export-view__hint export-view__hint--warn">
          {{ t('export.oneTableHint') }}
        </span>
        <n-checkbox
          v-for="table in tableOptions"
          :key="table.name"
          :checked="selected.includes(table.name)"
          @update:checked="(checked: boolean) => toggleTable(table.name, checked)"
        >
          {{ table.name }}
        </n-checkbox>
      </div>

      <n-button
        type="primary"
        :disabled="submitDisabled"
        :loading="exporting"
        data-testid="export-submit"
        @click="submit"
      >
        {{ t('export.submit') }}
      </n-button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { NAlert, NButton, NCheckbox, NSelect, useNotification, type SelectOption } from 'naive-ui'

import type { ExportRequest } from '../api/types'
import { downloadExport } from '../lib/download'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'

const { t } = useI18n()
const route = useRoute()
const catalog = useCatalogStore()
const session = useSessionStore()
const notification = useNotification()

const format = ref('sql')
const mode = ref<'all' | 'structure' | 'data'>('all')
const dropIfExists = ref(false)
const ifNotExists = ref(false)
const gzip = ref(false)
const selected = ref<string[]>([])
const exporting = ref(false)

const db = computed(() => String(route.params.db ?? ''))
const includeViews = ref(true)
const isSql = computed(() => format.value === 'sql')
const dbMismatch = computed(() => session.sessionInfo?.driver.flavor === 'postgres' && session.sessionInfo?.database !== db.value)
// Delimited formats dump one single-file table; sql may take the whole
// database (empty selection).
const submitDisabled = computed(
  () => dbMismatch.value || exporting.value || (!isSql.value && selected.value.length !== 1),
)

const tableOptions = computed(() => catalog.tables.filter((table) => !table.isView || (isSql.value && mode.value !== 'data' && includeViews.value)))

watch([format, mode, includeViews, tableOptions], () => {
  const valid = new Set(tableOptions.value.map(table => table.name))
  selected.value = selected.value.filter(name => valid.has(name))
})

const formatOptions = computed<SelectOption[]>(() => [
  { label: t('export.formatSql'), value: 'sql' },
  { label: t('export.formatCsv'), value: 'csv' },
  { label: t('export.formatCsvSemicolon'), value: 'csv_semicolon' },
  { label: t('export.formatTsv'), value: 'tsv' },
])

const modeOptions = computed<SelectOption[]>(() => ['all', 'structure', 'data'].map(value => ({ value, label: t(`transferWorkflows.mode_${value}`) })))

watch([db, () => catalog.schema, () => route.query?.table], async (_value, _old, onCleanup) => {
  let stale = false
  onCleanup(() => { stale = true })
  selected.value = []
  const requested = route.query?.table
  await catalog.fetchTables(db.value)
  if (!stale && typeof requested === 'string' && tableOptions.value.some(table => table.name === requested)) selected.value = [requested]
}, { immediate: true })

function toggleTable(name: string, checked: boolean): void {
  if (checked) {
    selected.value = [...selected.value, name]
  } else {
    selected.value = selected.value.filter((existing) => existing !== name)
  }
}

async function submit(): Promise<void> {
  if (submitDisabled.value) return
  exporting.value = true
  try {
    const body: ExportRequest = {
      format: format.value,
      ...(isSql.value ? { mode: mode.value } : {}),
      tables: selected.value,
      // The sql-only flags stay false for delimited formats (their
      // checkboxes are hidden); the server ignores them there anyway.
      includeViews: isSql.value && mode.value !== 'data' && includeViews.value,
      dropIfExists: isSql.value && mode.value !== 'data' && dropIfExists.value,
      ifNotExists: isSql.value && mode.value !== 'data' && ifNotExists.value,
      gzip: gzip.value,
    }
    await downloadExport(db.value, body, catalog.schemaAware ? catalog.schema : undefined)
    notification.success({ content: t('export.success'), duration: 4000 })
  } catch (err) {
    notification.error({
      content: err instanceof Error ? err.message : String(err),
      duration: 6000,
    })
  } finally {
    exporting.value = false
  }
}
</script>

<style scoped>
.export-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.export-view__form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 480px;
}

.export-view__field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

.export-view__hint {
  font-size: 12px;
  opacity: 0.7;
}

.export-view__hint--warn {
  color: #d97706;
  opacity: 1;
}
</style>

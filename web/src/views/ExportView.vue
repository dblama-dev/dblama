<template>
  <section class="export-view db-page tool-workspace">
    <ToolHeading icon="download" :title="t('export.title')" />
    <ui-alert v-if="dbMismatch" type="warning" data-testid="export-db-mismatch">
      {{ t('export.dbMismatch') }}
    </ui-alert>
    <div class="tool-card export-view__form">
      <div class="tool-card__body tool-stack">
        <div class="tool-fields">
          <label class="tool-field">
            {{ t('export.format') }}
            <ui-select v-model:value="format" :options="formatOptions" data-testid="export-format" />
          </label>
          <label v-if="isSql" class="tool-field">
            {{ t('transferWorkflows.mode') }}
            <ui-select v-model:value="mode" :options="modeOptions" data-testid="export-mode" />
          </label>
        </div>
        <p v-if="isSql && mode === 'data' && catalog.schemaAware" class="tool-note">
          {{ t('transferWorkflows.dataDependencies') }}
        </p>
        <p v-if="!isSql" class="tool-note">{{ t('transferWorkflows.csvExportEncoding') }}</p>
        <div class="export-view__flags">
          <template v-if="isSql && mode !== 'data'">
            <ui-checkbox v-model:checked="includeViews">{{ t('export.includeViews') }}</ui-checkbox>
            <ui-checkbox v-model:checked="dropIfExists">{{ t('export.dropIfExists') }}</ui-checkbox>
            <ui-checkbox v-model:checked="ifNotExists">{{ t('export.ifNotExists') }}</ui-checkbox>
          </template>
          <ui-checkbox v-model:checked="gzip">{{ t('export.gzip') }}</ui-checkbox>
        </div>
        <section class="export-view__tables tool-stack">
          <div>
            <h2>{{ t('export.tables') }}</h2>
            <p v-if="isSql" class="tool-note">{{ t('export.tablesHint') }}</p>
            <p v-else-if="selected.length !== 1" class="tool-note">{{ t('export.oneTableHint') }}</p>
          </div>
          <div class="table-options">
            <ui-checkbox
              v-for="table in tableOptions"
              :key="table.name"
              :checked="selected.includes(table.name)"
              @update:checked="(checked: boolean) => toggleTable(table.name, checked)"
            >
              {{ table.name }}
            </ui-checkbox>
          </div>
        </section>
      </div>
      <footer class="tool-card__foot">
        <ui-button
          type="primary"
          :disabled="submitDisabled"
          :loading="exporting"
          data-testid="export-submit"
          @click="submit"
        >
          <template #icon><UiIcon name="download" /></template>
          {{ t('export.submit') }}
        </ui-button>
        <span class="tool-note transfer-scope">
          {{
            t('transferWorkflows.context', {
              db,
              table: selected.join(', ') || t('transferWorkflows.allTables'),
            })
          }}
        </span>
      </footer>
    </div>
  </section>
</template>

<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { UiAlert, UiButton, UiCheckbox, UiSelect, useNotification, type SelectOption } from '../ui'

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
.export-view__form {
  max-width: 1040px;
}
.export-view__flags {
  display: flex;
  gap: 8px 18px;
  flex-wrap: wrap;
}
.export-view__tables {
  padding-top: 18px;
  border-top: 1px solid var(--db-line);
}
.table-options {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(185px, 100%), 1fr));
  gap: 8px;
  max-height: 310px;
  overflow: auto;
}
.table-options :deep(.ui-checkbox__label) {
  font-family: var(--db-mono);
  font-size: 13px;
  overflow-wrap: anywhere;
}
.transfer-scope {
  overflow-wrap: anywhere;
}
@media (pointer: coarse) {
  .table-options { gap: 2px 8px; }
  .export-view__form :deep(.ui-checkbox) { min-height: 38px; }
}
</style>

<template>
  <section class="import-view">
    <h2>{{ t('import.title') }}</h2>

    <p>{{ t('transferWorkflows.context', { db, table: table || t('transferWorkflows.allTables') }) }}</p>
    <n-alert v-if="result" :type="result === 'success' ? 'success' : 'warning'" data-testid="import-result">
      {{ t('transferWorkflows.context', resultContext) }}
      {{ result === 'success' ? t('import.success') : t(result === 'cancelled' ? 'transferWorkflows.cancelled' : 'transferWorkflows.failed') }}
      <template v-if="result !== 'success'">
        {{ t(resultType === 'csv' ? 'transferWorkflows.completedRows' : 'transferWorkflows.completedBytes', { done }) }}
        {{ t('transferWorkflows.partial') }} {{ resultError }}
      </template>
    </n-alert>
    <div class="import-view__form">
      <label class="import-view__field">
        <span>{{ t('import.file') }}</span>
        <input
          type="file"
          :disabled="importing"
          data-testid="import-file"
          @change="onFileChange"
        />
      </label>

      <div class="import-view__field">
        <span>{{ t('import.type') }}</span>
        <n-radio-group :disabled="importing" :value="type" @update:value="onTypeChange">
          <n-radio value="sql">{{ t('import.typeSql') }}</n-radio>
          <n-radio value="csv">{{ t('import.typeCsv') }}</n-radio>
        </n-radio-group>
      </div>

      <template v-if="type === 'csv'">
        <label class="import-view__field">
          <span>{{ t('import.table') }}</span>
          <n-select
            v-model:value="table"
            :disabled="importing"
            :options="tableOptions"
            clearable
            data-testid="import-table"
          />
        </label>
        <label class="import-view__field">
          <span>{{ t('transferWorkflows.preset') }}</span>
          <n-select :value="separator" :disabled="importing" :options="presetOptions" data-testid="import-preset" @update:value="applyPreset" />
        </label>
        <label class="import-view__field">
          <span>{{ t('import.separator') }}</span>
          <n-input v-model:value="separator" :disabled="importing" data-testid="import-separator" />
        </label>
        <label class="import-view__field">
          <span>{{ t('import.enclosure') }}</span>
          <n-input v-model:value="enclosure" :disabled="importing" data-testid="import-enclosure" />
        </label>
        <n-checkbox v-model:checked="skipHeader" :disabled="importing">
          {{ t('import.skipHeader') }}
        </n-checkbox>
        <n-alert v-if="previewError" type="error" data-testid="csv-preview-error">{{ t('transferWorkflows.previewError') }} {{ previewError }}</n-alert>
        <p v-if="previewLoading || columnsLoading">{{ t('transferWorkflows.previewLoading') }}</p>
        <template v-if="previewRows.length && !previewError">
          <p>{{ t('transferWorkflows.previewHint') }}</p>
          <n-checkbox v-model:checked="mappingEnabled" :disabled="importing" data-testid="csv-mapping-enabled">{{ t('transferWorkflows.mapping') }}</n-checkbox>
          <p v-if="mappingEnabled">{{ t('transferWorkflows.mappingHint') }}</p>
          <div class="import-view__preview" data-testid="csv-preview">
            <table>
              <thead><tr><th v-for="(name, index) in previewRows[0]" :key="index">
                {{ skipHeader ? name : t('transferWorkflows.sourceColumn', { number: index + 1 }) }}
                <n-select v-if="mappingEnabled" :value="mapping[index]" :disabled="importing" :options="columnOptions" :data-testid="`csv-map-${index}`" @update:value="(value: string) => mapping[index] = value" />
              </th></tr></thead>
              <tbody><tr v-for="(row, index) in dataPreviewRows" :key="index"><td v-for="(value, field) in row" :key="field" :title="value">{{ value.length > 160 ? value.slice(0, 160) + '…' : value }}</td></tr></tbody>
            </table>
          </div>
          <p v-if="mappingError" role="alert">{{ mappingError }}</p>
        </template>
      </template>

      <!-- The read phase of an SQL import reports no progress (the server
           emits execution-position lines only), so the bar stays
           indeterminate until the first NDJSON line arrives. -->
      <div
        v-if="importing || progressStarted"
        class="import-view__field"
        data-testid="import-progress"
      >
        <n-progress
          type="line"
          :percentage="percentage"
          :indeterminate="importing && total === 0"
          :show-indicator="progressStarted && total > 0"
        />
      </div>

      <n-button v-if="importing" data-testid="import-stop" @click="stop">{{ t('transferWorkflows.stop') }}</n-button>
      <n-button
        type="primary"
        :disabled="submitDisabled"
        :loading="importing"
        data-testid="import-submit"
        @click="submit"
      >
        {{ t('import.submit') }}
      </n-button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
  NAlert,
  NButton,
  NCheckbox,
  NInput,
  NProgress,
  NRadio,
  NRadioGroup,
  NSelect,
  useNotification,
  type SelectOption,
} from 'naive-ui'

import { buildImportFormData, runImport } from '../lib/importFile'
import { CSV_PREVIEW_BYTES, parseCsvPreview, readCsvPreview } from '../lib/csvPreview'
import { api } from '../api/client'
import type { ColumnDef, TableDef } from '../api/types'
import { useCatalogStore } from '../stores/catalog'

const { t } = useI18n()
const route = useRoute()
const catalog = useCatalogStore()
const notification = useNotification()

const type = ref<'sql' | 'csv'>('sql')
const table = ref<string | null>(null)
const separator = ref(',')
const enclosure = ref('')
const skipHeader = ref(false)
const file = ref<File | null>(null)
const importing = ref(false)
const done = ref(0)
const total = ref(0)
const progressStarted = ref(false)
const result = ref<'success' | 'cancelled' | 'error' | null>(null)
const resultError = ref('')
const resultType = ref<'sql' | 'csv'>('sql')
const resultContext = ref({ db: '', table: '' })
const previewRows = ref<string[][]>([])
const previewError = ref('')
const previewLoading = ref(false)
const columnsLoading = ref(false)
const columns = ref<ColumnDef[]>([])
const columnsError = ref('')
const mapping = ref<string[]>([])
const mappingEnabled = ref(true)
let controller: AbortController | null = null
let previewID = 0
let columnsID = 0

const db = computed(() => String(route.params.db ?? ''))

const tableOptions = computed<SelectOption[]>(() =>
  catalog.tables
    .filter((item) => !item.isView)
    .map((item) => ({ label: item.name, value: item.name })),
)

// CSV needs a target table and a valid bounded preview. SQL needs a file.
const submitDisabled = computed(
  () => importing.value || file.value === null || (type.value === 'csv' && (!table.value || previewLoading.value || columnsLoading.value || !!previewError.value || !!mappingError.value)),
)

const percentage = computed(() =>
  total.value > 0 ? Math.min(100, Math.round((done.value / total.value) * 100)) : 0,
)

const presetOptions = computed<SelectOption[]>(() => [
  { value: ',', label: t('transferWorkflows.commaPreset') },
  { value: ';', label: t('transferWorkflows.semicolonPreset') },
  { value: '\t', label: t('transferWorkflows.tabPreset') },
])
const columnOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('transferWorkflows.ignoreColumn') },
  ...columns.value.map(column => ({ value: column.name, label: column.name })),
])
const dataPreviewRows = computed(() => (skipHeader.value ? previewRows.value.slice(1) : previewRows.value).slice(0, 20))
const mappingError = computed(() => {
  if (!mappingEnabled.value) return ''
  if (columnsError.value) return columnsError.value
  if (columnsLoading.value || !previewRows.value.length) return ''
  const targets = mapping.value.filter(Boolean)
  if (!targets.length) return t('transferWorkflows.mappingRequired')
  if (new Set(targets).size !== targets.length) return t('transferWorkflows.mappingDuplicate')
  return ''
})
function applyPreset(value: string): void { separator.value = value; enclosure.value = '"' }
function resetMapping(): void {
  const first = previewRows.value[0] ?? []
  mapping.value = first.map((name, index) => {
    if (skipHeader.value) return columns.value.some(column => column.name === name) ? name : ''
    return columns.value[index]?.name ?? ''
  })
}
watch([db, () => catalog.schema, () => route.query?.table], async (_value, _old, onCleanup) => {
  let stale = false
  onCleanup(() => { stale = true })
  stop(); table.value = null
  const requested = route.query?.table
  await catalog.fetchTables(db.value)
  if (!stale && typeof requested === 'string' && catalog.tables.some(item => item.name === requested && !item.isView)) table.value = requested
}, { immediate: true })
watch([db, () => catalog.schema, table], async () => {
  const id = ++columnsID; columns.value = []; columnsError.value = ''
  if (!table.value) { columnsLoading.value = false; resetMapping(); return }
  columnsLoading.value = true
  try {
    const def = await api<TableDef>('GET', catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables/${encodeURIComponent(table.value)}`))
    if (id === columnsID) columns.value = def.columns
  } catch (err) {
    if (id === columnsID) columnsError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (id === columnsID) { columnsLoading.value = false; resetMapping() }
  }
})
watch([file, type, separator, enclosure], async () => {
  const id = ++previewID; previewRows.value = []; previewError.value = ''
  if (!file.value || type.value !== 'csv') { previewLoading.value = false; return }
  previewLoading.value = true
  const selectedFile = file.value
  try {
    const text = await readCsvPreview(selectedFile)
    const parsed = parseCsvPreview(text, separator.value, enclosure.value, selectedFile.size > CSV_PREVIEW_BYTES)
    if (id !== previewID) return
    if (!parsed.rows.length) throw new Error(t('transferWorkflows.noPreviewRows'))
    previewRows.value = parsed.rows
    resetMapping()
  } catch (err) {
    if (id === previewID) previewError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (id === previewID) previewLoading.value = false
  }
})
watch(skipHeader, resetMapping)
onBeforeUnmount(() => { stop(); previewID++; columnsID++ })
function stop(): void { controller?.abort() }

function onTypeChange(value: 'sql' | 'csv'): void {
  type.value = value
}

function onFileChange(event: Event): void {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
}

async function submit(): Promise<void> {
  if (submitDisabled.value || file.value === null) return
  controller = new AbortController()
  const activeController = controller
  const operationDB = db.value
  const operationSchema = catalog.schema
  resultType.value = type.value
  resultContext.value = { db: operationDB, table: table.value || t('transferWorkflows.allTables') }
  importing.value = true
  result.value = null
  resultError.value = ''
  done.value = 0
  total.value = 0
  progressStarted.value = false
  try {
    const form = buildImportFormData(type.value, file.value, {
      table: table.value ?? '',
      separator: separator.value,
      enclosure: enclosure.value,
      skipHeader: skipHeader.value,
      ...(mappingEnabled.value && type.value === 'csv' ? { columns: [...mapping.value] } : {}),
    })
    await runImport(operationDB, form, (lineDone, lineTotal) => {
      progressStarted.value = true
      done.value = lineDone
      total.value = lineTotal
    }, catalog.schemaAware ? operationSchema : undefined, activeController.signal)
    result.value = 'success'
    notification.success({ content: t('import.success'), duration: 4000 })
  } catch (err) {
    result.value = activeController.signal.aborted ? 'cancelled' : 'error'
    resultError.value = result.value === 'cancelled' ? '' : (err instanceof Error ? err.message : String(err))
    if (result.value === 'error') notification.error({ content: resultError.value, duration: 6000 })
  } finally {
    importing.value = false
    controller = null
    // Even failed or cancelled requests may have applied earlier batches.
    if (db.value === operationDB && catalog.schema === operationSchema) await catalog.invalidate(operationDB)
  }
}
</script>

<style scoped>
.import-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.import-view__form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 900px;
}

.import-view__field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.import-view__preview { max-width: 100%; overflow: auto; }
.import-view__preview table { border-collapse: collapse; }
.import-view__preview th, .import-view__preview td { border: 1px solid #8885; padding: 8px; min-width: 180px; max-width: 280px; white-space: pre-wrap; overflow-wrap: anywhere; vertical-align: top; }
.import-view__preview th { text-align: left; }
</style>

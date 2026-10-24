<template>
  <section class="import-view">
    <h2>{{ t('import.title') }}</h2>

    <div class="import-view__form">
      <label class="import-view__field">
        <span>{{ t('import.file') }}</span>
        <input
          type="file"
          data-testid="import-file"
          @change="onFileChange"
        />
      </label>

      <div class="import-view__field">
        <span>{{ t('import.type') }}</span>
        <n-radio-group :value="type" @update:value="onTypeChange">
          <n-radio value="sql">{{ t('import.typeSql') }}</n-radio>
          <n-radio value="csv">{{ t('import.typeCsv') }}</n-radio>
        </n-radio-group>
      </div>

      <template v-if="type === 'csv'">
        <label class="import-view__field">
          <span>{{ t('import.table') }}</span>
          <n-select
            v-model:value="table"
            :options="tableOptions"
            clearable
            data-testid="import-table"
          />
        </label>
        <label class="import-view__field">
          <span>{{ t('import.separator') }}</span>
          <n-input v-model:value="separator" data-testid="import-separator" />
        </label>
        <label class="import-view__field">
          <span>{{ t('import.enclosure') }}</span>
          <n-input v-model:value="enclosure" data-testid="import-enclosure" />
        </label>
        <n-checkbox v-model:checked="skipHeader">
          {{ t('import.skipHeader') }}
        </n-checkbox>
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
          :indeterminate="importing && !progressStarted"
          :show-indicator="progressStarted"
        />
      </div>

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
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
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

const db = computed(() => String(route.params.db ?? ''))

const tableOptions = computed<SelectOption[]>(() =>
  catalog.tables
    .filter((item) => !item.isView)
    .map((item) => ({ label: item.name, value: item.name })),
)

// csv imports need a target table (server contract) plus a file; sql
// imports only the file. Note: unlike export there is no login-database
// restriction here — the import endpoint honors the {db} URL parameter.
const submitDisabled = computed(
  () => importing.value || file.value === null || (type.value === 'csv' && (table.value === null || table.value === '')),
)

const percentage = computed(() =>
  total.value > 0 ? Math.min(100, Math.round((done.value / total.value) * 100)) : 0,
)

onMounted(() => {
  void catalog.fetchTables(db.value)
})

function onTypeChange(value: 'sql' | 'csv'): void {
  type.value = value
}

function onFileChange(event: Event): void {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
}

async function submit(): Promise<void> {
  if (submitDisabled.value || file.value === null) return
  importing.value = true
  done.value = 0
  total.value = 0
  progressStarted.value = false
  try {
    const form = buildImportFormData(type.value, file.value, {
      table: table.value ?? '',
      separator: separator.value,
      enclosure: enclosure.value,
      skipHeader: skipHeader.value,
    })
    await runImport(db.value, form, (lineDone, lineTotal) => {
      progressStarted.value = true
      done.value = lineDone
      total.value = lineTotal
    })
    notification.success({ content: t('import.success'), duration: 4000 })
    // The imported tables (and their sizes) may have changed.
    await catalog.invalidate(db.value)
  } catch (err) {
    notification.error({
      content: err instanceof Error ? err.message : String(err),
      duration: 6000,
    })
  } finally {
    importing.value = false
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
  max-width: 480px;
}

.import-view__field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
</style>

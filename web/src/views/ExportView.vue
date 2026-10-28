<template>
  <section class="export-view">
    <h2>{{ t('export.title') }}</h2>

    <!-- The backend export endpoint dumps the login database only, so
         exporting a different database is impossible — block it early. -->
    <n-alert v-if="dbMismatch" type="warning" data-testid="export-db-mismatch">
      {{ t('export.dbMismatch') }}
    </n-alert>

    <div class="export-view__form">
      <label class="export-view__field">
        <span>{{ t('export.format') }}</span>
        <n-select
          v-model:value="format"
          :options="formatOptions"
          data-testid="export-format"
        />
      </label>

      <div v-if="isSql" class="export-view__field">
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
import { computed, onMounted, ref } from 'vue'
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
const dropIfExists = ref(false)
const ifNotExists = ref(false)
const gzip = ref(false)
const selected = ref<string[]>([])
const exporting = ref(false)

const db = computed(() => String(route.params.db ?? ''))
const isSql = computed(() => format.value === 'sql')
// The export endpoint dumps the driver's login database; the {db} URL
// parameter is ignored there (unlike import), so a mismatch means the
// user cannot get what the dialog seems to offer.
const dbMismatch = computed(() => {
  const login = session.sessionInfo?.database
  return login !== null && login !== undefined && login !== db.value
})
// Delimited formats dump one single-file table; sql may take the whole
// database (empty selection).
const submitDisabled = computed(
  () => dbMismatch.value || exporting.value || (!isSql.value && selected.value.length !== 1),
)

const tableOptions = computed(() => catalog.tables.filter((table) => !table.isView))

const formatOptions = computed<SelectOption[]>(() => [
  { label: t('export.formatSql'), value: 'sql' },
  { label: t('export.formatCsv'), value: 'csv' },
  { label: t('export.formatCsvSemicolon'), value: 'csv_semicolon' },
  { label: t('export.formatTsv'), value: 'tsv' },
])

onMounted(() => {
  void catalog.fetchTables(db.value)
})

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
      tables: selected.value,
      // The sql-only flags stay false for delimited formats (their
      // checkboxes are hidden); the server ignores them there anyway.
      dropIfExists: isSql.value && dropIfExists.value,
      ifNotExists: isSql.value && ifNotExists.value,
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

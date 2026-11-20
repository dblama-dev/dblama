<template>
  <section class="resultgrid">
    <header class="resultgrid__header">
      <span class="resultgrid__index">#{{ index }}</span>
      <n-tag v-if="result.truncated" size="small" type="warning" round>
        {{ t('console.truncated') }}
      </n-tag>
      <n-button v-if="columns.length" class="resultgrid__download" size="small" @click="saveCSV">
        {{ t(result.truncated ? 'console.downloadDisplayedCSV' : 'console.downloadCSV') }}
      </n-button>
    </header>

    <p v-if="affectedMessage !== null" class="resultgrid__affected">
      {{ affectedMessage }}
    </p>
    <n-table v-else-if="columns.length > 0" size="small" :single-line="false">
      <thead>
        <tr>
          <th v-for="(meta, position) in columns" :key="position">{{ meta.name }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, i) in rows" :key="i">
          <td v-for="(cell, j) in row" :key="j">
            <span
              v-if="cell === null || cell === undefined"
              class="resultgrid__null"
              style="font-style: italic"
            >{{ t('grid.null') }}</span>
            <template v-else>{{ cell }}</template>
          </td>
        </tr>
      </tbody>
    </n-table>

    <footer class="resultgrid__footer">
      {{ t('console.seconds', { n: result.seconds.toFixed(2) }) }}
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTable, NTag } from 'naive-ui'

import type { QueryResult } from '../api/types'
import { downloadBlob } from '../lib/download'
import { resultCsv } from '../lib/resultCsv'

// Pure presentational component: one statement's Result of a console
// batch. Rows render like DataGrid (italic NULL placeholders); a
// non-row statement shows the affected-rows message instead of a table.
const props = defineProps<{
  result: QueryResult
  /** 1-based statement number shown as #N. */
  index: number
}>()

const { t } = useI18n()

// Go's nil slices serialize as null for DDL/DML and empty SELECT results.
const columns = computed(() => props.result.columns ?? [])
const rows = computed(() => props.result.rows ?? [])

function saveCSV(): void {
  const suffix = props.result.truncated ? '-truncated' : ''
  downloadBlob(resultCsv(props.result), `query-result-${props.index}${suffix}.csv`)
}

// A statement without rows reports what it changed instead of a table.
const affectedMessage = computed<string | null>(() => {
  const affected = props.result.affectedRows ?? 0
  return rows.value.length === 0 && affected > 0
    ? t('console.rowsAffected', { n: affected })
    : null
})
</script>

<style scoped>
.resultgrid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 0;
  border-top: 1px solid rgba(128, 128, 128, 0.25);
}

.resultgrid:first-child {
  border-top: none;
  padding-top: 0;
}

.resultgrid__header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.resultgrid__index {
  font-weight: 600;
  opacity: 0.8;
}

.resultgrid__download { margin-inline-start: auto; }

.resultgrid__null {
  opacity: 0.6;
}

.resultgrid__footer {
  opacity: 0.7;
  font-size: 12px;
}
</style>

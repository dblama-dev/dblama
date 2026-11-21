<template>
  <section class="resultgrid">
    <header class="resultgrid__header">
      <span class="resultgrid__index">{{ t('rowWorkflows.result', { n: index }) }}</span>
      <span v-if="result.truncated" class="db-tag db-tag--warning">
        {{ t('console.truncated') }}
      </span>
      <ui-button v-if="columns.length" class="resultgrid__download" size="small" @click="saveCSV">
        <template #icon><UiIcon name="download" :size="14" /></template>
        {{ t(result.truncated ? 'console.downloadDisplayedCSV' : 'console.downloadCSV') }}
      </ui-button>
    </header>

    <p v-if="affectedMessage !== null" class="resultgrid__affected">
      {{ affectedMessage }}
    </p>
    <div v-else-if="columns.length" class="resultgrid__scroll" tabindex="0" :aria-label="t('rowWorkflows.result', { n: index })">
    <table class="db-table">
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
    </table>
    </div>

    <footer class="resultgrid__footer">
      {{ t('console.seconds', { n: result.seconds.toFixed(2) }) }}
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import UiIcon from './UiIcon.vue'
import { UiButton } from '../ui'

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
.resultgrid { min-width:0; overflow:hidden; border:1px solid var(--db-line); border-radius:10px; background:var(--db-panel); }
.resultgrid__header { display:flex; align-items:center; gap:8px; padding:10px 14px; border-bottom:1px solid var(--db-line); }
.resultgrid__index { font-weight:600; }
.resultgrid__download { margin-inline-start:auto; }
.resultgrid__scroll { overflow:auto; max-height:55vh; }
.resultgrid__scroll th { font:550 12px var(--db-mono); }
.resultgrid__scroll td { max-width:45ch; overflow-wrap:anywhere; }
.resultgrid__null { color:var(--db-muted); font-family:var(--db-mono); }
.resultgrid__footer { padding:8px 14px; color:var(--db-muted); border-top:1px solid var(--db-line); font-size:12px; }
.resultgrid__affected { margin:0; padding:14px; }
</style>

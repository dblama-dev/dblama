<template>
  <section class="search-view">
    <h2>{{ t('searchWorkflows.title') }}: {{ db }}<small v-if="catalog.schema"> / {{ catalog.schema }}</small></h2>
    <p><a :href="databaseLink" @click.prevent="router.push(databaseLink)">{{ t('searchWorkflows.database') }}: {{ db }}</a></p>
    <n-alert type="info">{{ t('searchWorkflows.limits') }}<br />{{ t('searchWorkflows.semantics') }}</n-alert>
    <n-alert v-if="catalog.error || error" type="error">{{ catalog.error || error }}</n-alert>
    <form class="search-view__form" @submit.prevent="start">
      <label>{{ t('searchWorkflows.text') }}<input v-model="text" data-search-text type="text" :disabled="busy" :aria-label="t('searchWorkflows.text')" /></label>
      <fieldset :disabled="busy || catalog.loading">
        <legend>{{ t('searchWorkflows.tables') }}</legend>
        <div class="search-view__tables">
          <label v-for="table in catalog.tables" :key="table.name"><input v-model="selected" type="checkbox" :value="table.name" :disabled="!selected.includes(table.name) && selected.length >= 20" />{{ table.name }}</label>
        </div>
        <p v-if="!catalog.loading && !catalog.tables.length">{{ t('searchWorkflows.noCatalog') }}</p>
      </fieldset>
      <p>{{ t('searchWorkflows.selected', { count: selected.length }) }}</p>
      <n-space><n-button attr-type="submit" type="primary" :disabled="busy || catalog.loading">{{ t('searchWorkflows.start') }}</n-button><n-button v-if="busy" data-search-stop @click="stop">{{ t('searchWorkflows.stop') }}</n-button></n-space>
    </form>
    <div v-if="status !== 'idle'" role="status" aria-live="polite">
      <p>{{ t(`searchWorkflows.${status}`) }}</p>
      <p>{{ t('searchWorkflows.progress', { done: progress.completedTables, total: progress.totalTables, columns: progress.searchedColumns }) }}</p>
    </div>
    <n-alert v-if="errors.length" type="warning" :title="t('searchWorkflows.errors')"><ul><li v-for="(item, index) in errors" :key="index">{{ item.table }}<span v-if="item.column"> / {{ item.column }}</span>: {{ localError(item.message) }}</li></ul></n-alert>
    <p v-if="status === 'complete' && !results.length && !errors.length">{{ t('searchWorkflows.noResults') }}</p>
    <section v-for="result in results" :key="JSON.stringify([result.table,result.column])" class="search-view__result">
      <h3>{{ result.table }} / {{ result.column }}</h3>
      <p>{{ t('searchWorkflows.showing', { count: result.rows.length, total: result.totalRows }) }}</p>
      <a data-search-result :href="result.href" @click.prevent="router.push(result.href)">{{ t('searchWorkflows.open') }}</a>
      <ol :aria-label="t('searchWorkflows.results')"><li v-for="(row, index) in result.rows" :key="index"><pre>{{ preview(row[result.columns.findIndex(column => column.name === result.column)]) }}</pre></li></ol>
    </section>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NSpace } from 'naive-ui'
import { useCatalogStore } from '../stores/catalog'
import { searchDatabase, type SearchError, type SearchProgress, type SearchResult } from '../lib/databaseSearch'
const route = useRoute(), router = useRouter(), catalog = useCatalogStore()
const { t } = useI18n()
const db = computed(() => String(route.params.db ?? ''))
const databaseLink = computed(() => catalog.path(`/db/${encodeURIComponent(db.value)}`))
const selected = ref<string[]>([]), text = ref(''), error = ref(''), busy = ref(false)
const status = ref<'idle'|'running'|'complete'|'cancelled'>('idle')
const results = ref<SearchResult[]>([]), errors = ref<SearchError[]>([])
const progress = ref<SearchProgress>({ completedTables: 0, totalTables: 0, searchedColumns: 0 })
let controller: AbortController | undefined, run = 0
function localError(message: string): string { return ['emptyQuery','tooManyTables','noTables','requestTimeout'].includes(message) ? t(`searchWorkflows.${message}`) : message }
function preview(value: unknown): string { const content = value === null ? 'NULL' : typeof value === 'string' ? value : JSON.stringify(value) ?? ''; return content.length > 240 ? `${content.slice(0,240)}…` : content }
function stop(): void { controller?.abort() }
async function start(): Promise<void> {
  if (busy.value) return
  const id = ++run
  controller = new AbortController()
  busy.value = true; error.value = ''; results.value = []; errors.value = []; status.value = 'running'
  try {
    const summary = await searchDatabase({ db: db.value, schema: catalog.schema, tables: [...selected.value], text: text.value, signal: controller.signal,
      onResult: result => { if (run === id) results.value.push(result) },
      onError: item => { if (run === id) errors.value.push(item) },
      onProgress: value => { if (run === id) progress.value = value },
    })
    if (run === id) status.value = summary.cancelled ? 'cancelled' : 'complete'
  } catch (err) { if (run === id) { error.value = localError(err instanceof Error ? err.message : String(err)); status.value = 'idle' } }
  finally { if (run === id) busy.value = false }
}
let pendingContext: { path: string; table: string | null; selection: string[] } | undefined
watch([db, () => catalog.schema, () => catalog.schemaAware, () => route.query.table], () => {
  stop(); run++; busy.value = false; status.value = 'idle'; selected.value = []; results.value = []; errors.value = []; error.value = ''
  const table = typeof route.query.table === 'string' ? route.query.table : null
  pendingContext = { path: catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables`), table, selection: selected.value }
  void catalog.fetchTables(db.value)
}, { immediate: true })
watch(() => [catalog.loading, catalog.tablesPath, catalog.tables, catalog.error], () => {
  const context = pendingContext
  if (!context || catalog.loading || catalog.error || catalog.tablesPath !== context.path) return
  pendingContext = undefined
  // Another loader may supersede our request in the same scope. Initialize
  // from its successful catalog once, without resetting manual selections.
  if (selected.value !== context.selection) return
  if (context.table && catalog.tables.some(item => item.name === context.table)) selected.value = [context.table]
})
onBeforeUnmount(() => { pendingContext = undefined; stop(); run++ })
</script>
<style scoped>
.search-view{display:flex;flex-direction:column;gap:12px;max-width:100%;}.search-view h2,.search-view p{margin:0;}.search-view__form{display:flex;flex-direction:column;gap:12px;}.search-view__form>label{display:flex;flex-direction:column;gap:6px;max-width:560px;}.search-view__form input[type=text]{font:inherit;padding:8px;border:1px solid #8888;border-radius:4px;background:transparent;color:inherit;}.search-view__tables{display:flex;gap:8px 20px;flex-wrap:wrap;max-height:220px;overflow:auto;}.search-view__tables label{display:flex;gap:6px;align-items:center;overflow-wrap:anywhere;}fieldset{border:1px solid #8888;border-radius:4px;min-width:0;}.search-view__result{border-top:1px solid #8885;padding-top:8px;min-width:0;}.search-view__result pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;}small{font-weight:normal;}
</style>

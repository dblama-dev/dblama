<template>
  <section class="search-view db-page tool-workspace">
    <ToolHeading icon="search" :title="t('searchWorkflows.title')" />
    <ui-alert v-if="catalog.error || error" type="error">{{ catalog.error || error }}</ui-alert>
    <form class="tool-card search-view__form" @submit.prevent="start">
      <div class="tool-card__body tool-stack">
        <label class="tool-field search-view__term">
          {{ t('searchWorkflows.text') }}
          <input
            v-model="text"
            data-search-text
            type="text"
            :disabled="busy"
            :aria-label="t('searchWorkflows.text')"
          />
        </label>
        <fieldset :disabled="busy || catalog.loading">
          <legend>{{ t('searchWorkflows.tables') }}</legend>
          <div class="search-view__tables">
            <label v-for="table in tables" :key="table.name">
              <input
                v-model="selected"
                type="checkbox"
                :value="table.name"
                :disabled="!selected.includes(table.name) && selected.length >= 20"
              />
              {{ table.name }}
            </label>
          </div>
          <p v-if="!catalog.loading && !tables.length" class="tool-note">
            {{ t('searchWorkflows.noCatalog') }}
          </p>
        </fieldset>
      </div>
      <footer class="tool-card__foot">
        <div class="tool-actions">
          <ui-button attr-type="submit" type="primary" :disabled="busy || catalog.loading">
            <template #icon><UiIcon name="search" /></template>
            {{ t('searchWorkflows.start') }}
          </ui-button>
          <ui-button v-if="busy" data-search-stop @click="stop">{{ t('searchWorkflows.stop') }}</ui-button>
        </div>
        <span class="tool-note">{{ t('searchWorkflows.selected', { count: selected.length }) }}</span>
      </footer>
    </form>
    <details class="tool-card tool-details">
      <summary>{{ t('searchWorkflows.options') }}</summary>
      <div class="tool-card__body tool-note">
        <p>{{ t('searchWorkflows.limits') }}</p>
        <p>{{ t('searchWorkflows.semantics') }}</p>
      </div>
    </details>
    <div v-if="status !== 'idle'" role="status" aria-live="polite">
      <h2>{{ t(`searchWorkflows.${status}`) }}</h2>
      <p class="tool-note">
        {{
          t('searchWorkflows.progress', {
            done: progress.completedTables,
            total: progress.totalTables,
            columns: progress.searchedColumns,
          })
        }}
      </p>
    </div>
    <ui-alert v-if="errors.length" type="warning" :title="t('searchWorkflows.errors')">
      <ul>
        <li v-for="(item, index) in errors" :key="index">
          {{ item.table }}
          <span v-if="item.column">/ {{ item.column }}</span>
          · {{ localError(item.message) }}
        </li>
      </ul>
    </ui-alert>
    <p v-if="status === 'complete' && !results.length && !errors.length" class="tool-card tool-empty">
      {{ t('searchWorkflows.noResults') }}
    </p>
    <section
      v-for="result in results"
      :key="JSON.stringify([result.table, result.column])"
      class="tool-card search-view__result"
    >
      <header class="tool-card__head">
        <div>
          <h3>{{ result.table }} / {{ result.column }}</h3>
          <p class="tool-note">
            {{ t('searchWorkflows.showing', { count: result.rows.length, total: result.totalRows }) }}
          </p>
        </div>
        <a data-search-result :href="result.href" @click.prevent="router.push(result.href)">
          {{ t('searchWorkflows.open') }}
          <UiIcon name="arrow-up-right" :size="14" />
        </a>
      </header>
      <ol :aria-label="t('searchWorkflows.results')">
        <li v-for="(row, index) in result.rows" :key="index">
          <pre>{{ preview(row[result.columns.findIndex((column) => column.name === result.column)]) }}</pre>
        </li>
      </ol>
    </section>
  </section>
</template>

<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { UiAlert, UiButton } from '../ui'
import { useCatalogStore } from '../stores/catalog'
import { searchDatabase, type SearchError, type SearchProgress, type SearchResult } from '../lib/databaseSearch'
const route = useRoute(), router = useRouter(), catalog = useCatalogStore()
const { t } = useI18n()
const db = computed(() => String(route.params.db ?? ''))
const tables = computed(() => catalog.tablesFor(db.value))
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
watch(() => [catalog.loading, catalog.tablesPath, tables.value, catalog.error], () => {
  const context = pendingContext
  if (!context || catalog.loading || catalog.error || catalog.tablesPath !== context.path) return
  pendingContext = undefined
  // Another loader may supersede our request in the same scope. Initialize
  // from its successful catalog once, without resetting manual selections.
  if (selected.value !== context.selection) return
  if (context.table && tables.value.some(item => item.name === context.table)) selected.value = [context.table]
})
onBeforeUnmount(() => { pendingContext = undefined; stop(); run++ })
</script>
<style scoped>
.search-view__term {
  max-width: 560px;
}
fieldset {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
legend {
  font-weight: 550;
  font-size: 13px;
  margin-bottom: 8px;
}
.search-view__tables {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(190px, 100%), 1fr));
  gap: 2px 12px;
  max-height: 300px;
  overflow: auto;
}
.search-view__tables label {
  min-height: 30px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 7px;
  font: 13px var(--db-mono);
  overflow-wrap: anywhere;
}
.search-view__result h3 {
  font-family: var(--db-mono);
}
.search-view__result ol {
  padding: 0 24px 0 42px;
  max-height: 250px;
  overflow: auto;
}
.search-view__result pre {
  font: 13px var(--db-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.search-view__result a {
  color: var(--db-accent);
}
@media (pointer: coarse) {
  .search-view__tables label { min-height: 38px; }
}
</style>

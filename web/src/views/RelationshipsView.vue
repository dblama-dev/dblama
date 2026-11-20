<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, useThemeVars } from 'naive-ui'
import { api } from '../api/client'
import type { ForeignKey } from '../api/types'
import { useCatalogStore } from '../stores/catalog'

interface Relationship extends ForeignKey { table: string }

const { t } = useI18n()
const theme = useThemeVars()
const route = useRoute()
const catalog = useCatalogStore()
const db = computed(() => String(route.params.db))
const path = computed(() => catalog.path(`/api/databases/${encodeURIComponent(db.value)}/relationships`))
const scopeName = computed(() => catalog.schema ? `${db.value} / ${catalog.schema}` : db.value)
const relations = ref<Relationship[]>([])
const filter = ref('')
const error = ref('')
const loading = ref(true)
let request = 0

async function load() {
  const id = ++request
  loading.value = true
  error.value = ''
  try {
    const result = await api<{ relationships: Relationship[] }>('GET', path.value)
    if (id === request) relations.value = result.relationships
  } catch (err) {
    if (id === request) error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (id === request) loading.value = false
  }
}

watch(path, () => {
  relations.value = []
  filter.value = ''
  void load()
}, { immediate: true })

const visible = computed(() => {
  const term = filter.value.trim().toLowerCase()
  return relations.value.filter(r => [r.table, r.refTable, r.name, r.refSchema ?? '', ...r.columns, ...r.refColumns]
    .some(value => value.toLowerCase().includes(term)))
})

function structure(table: string, otherScope?: string) {
  const schema = catalog.schemaAware ? otherScope || catalog.schema : null
  return {
    name: 'db-table',
    params: { db: (!catalog.schemaAware && otherScope) || db.value, t: table },
    query: { view: 'structure', ...(schema ? { schema } : {}) },
  }
}
</script>

<template>
  <section class="relationships">
    <header>
      <RouterLink class="scope-link" :to="catalog.path('/db/' + encodeURIComponent(db))">{{ scopeName }}</RouterLink>
      <div class="title-row">
        <h2>{{ t('relationships.title') }}</h2>
        <n-button size="small" :loading="loading" @click="load">{{ t('server.refresh') }}</n-button>
      </div>
    </header>
    <n-alert v-if="error" type="error" role="alert">{{ error }}</n-alert>
    <p v-else-if="loading" role="status">{{ t('relationships.loading') }}</p>
    <p v-else-if="!relations.length" class="empty">{{ t('relationships.empty') }}</p>
    <template v-else>
      <div class="filters">
        <input v-model="filter" type="search" :aria-label="t('relationships.filter')" :placeholder="t('relationships.placeholder')">
        <span role="status">{{ t('relationships.count', { visible: visible.length, total: relations.length }) }}</span>
      </div>
      <p v-if="!visible.length" class="empty">{{ t('relationships.noMatches') }}</p>
      <article v-for="r in visible" :key="JSON.stringify([r.table, r.name])" :aria-label="r.name">
        <header class="constraint"><h3>{{ r.name }}</h3>
          <span v-if="r.onDelete">ON DELETE <b>{{ r.onDelete }}</b></span>
          <span v-if="r.onUpdate">ON UPDATE <b>{{ r.onUpdate }}</b></span>
        </header>
        <div class="mapping">
          <div class="endpoints">
            <div><small>{{ t('relationships.from') }}</small><RouterLink :to="structure(r.table)">{{ r.table }}</RouterLink></div>
            <span aria-hidden="true">→</span>
            <div><small>{{ t('relationships.to') }}</small><RouterLink :to="structure(r.refTable, r.refSchema)">{{ r.refSchema ? r.refSchema + '.' : '' }}{{ r.refTable }}</RouterLink></div>
          </div>
          <div v-for="(column, i) in r.columns" :key="i" class="pair">
            <code>{{ column }}</code><span aria-hidden="true">→</span><code>{{ r.refColumns[i] }}</code>
          </div>
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.relationships { max-width: 1000px; margin: 0 auto; padding: 8px 0 32px; color: v-bind('theme.textColor1'); }
.title-row, .filters, .constraint { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 16px; }
.title-row { justify-content: space-between; margin: 6px 0 16px; }
h2, h3 { margin: 0; line-height: 1.3; }
h2 { font-size: 24px; }
h3 { font-size: 14px; margin-right: auto; }
a { color: v-bind('theme.primaryColorPressed'); text-underline-offset: 3px; overflow-wrap: anywhere; }
.scope-link { font-size: 13px; }
.filters { margin-bottom: 16px; }
.filters input { width: min(100%, 390px); padding: 8px 12px; border-radius: 5px; border: 1px solid v-bind('theme.borderColor'); background: v-bind('theme.inputColor'); color: inherit; font: inherit; }
.filters input:focus-visible { outline: 2px solid v-bind('theme.primaryColor'); outline-offset: 1px; }
.filters span, .constraint span, .empty, small { color: v-bind('theme.textColor2'); }
article { margin: 12px 0; border: 1px solid v-bind('theme.borderColor'); background: v-bind('theme.cardColor'); border-radius: 8px; overflow: hidden; }
.constraint { padding: 10px 16px; background: v-bind('theme.tableHeaderColor'); border-bottom: 1px solid v-bind('theme.borderColor'); overflow-wrap: anywhere; }
.constraint span { font-size: 11px; }
.constraint b { font-weight: 600; }
.mapping { padding: 12px 16px; }
.endpoints, .pair { display: grid; grid-template-columns: minmax(0,1fr) 16px minmax(0,1fr); align-items: center; gap: 10px; }
.endpoints { padding-bottom: 8px; }
.endpoints small { display: block; font-size: 12px; }
.endpoints a { font-size: 16px; font-weight: 600; }
.pair { font-size: 13px; padding: 3px 0; }
.pair code { overflow-wrap: anywhere; }
.pair span, .endpoints > span { color: v-bind('theme.textColor3'); }
.empty { padding: 22px; border: 1px dashed v-bind('theme.borderColor'); border-radius: 8px; }
</style>

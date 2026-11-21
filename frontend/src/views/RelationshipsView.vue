<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { UiAlert, UiButton } from '../ui'
import { api } from '../api/client'
import type { ForeignKey } from '../api/types'
import { useCatalogStore } from '../stores/catalog'

interface Relationship extends ForeignKey { table: string }

const { t } = useI18n()
const route = useRoute()
const catalog = useCatalogStore()
const db = computed(() => String(route.params.db))
const path = computed(() => catalog.path(`/api/databases/${encodeURIComponent(db.value)}/relationships`))
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
  <section class="relationships db-page tool-workspace">
    <ToolHeading icon="network" :title="t('relationships.title')">
      <ui-button size="small" :loading="loading" @click="load">
        <template #icon><UiIcon name="refresh" /></template>
        {{ t('server.refresh') }}
      </ui-button>
    </ToolHeading>
    <ui-alert v-if="error" type="error" role="alert">{{ error }}</ui-alert>
    <p v-else-if="loading" class="tool-card tool-empty" role="status">{{ t('relationships.loading') }}</p>
    <p v-else-if="!relations.length" class="tool-card tool-empty">{{ t('relationships.empty') }}</p>
    <div v-else class="tool-card relation-list">
      <div class="tool-card__head">
        <label class="relation-filter">
          <UiIcon name="search" :size="16" />
          <input
            v-model="filter"
            type="search"
            :aria-label="t('relationships.filter')"
            :placeholder="t('relationships.placeholder')"
          />
        </label>
        <span role="status" class="tool-note">
          {{ t('relationships.count', { visible: visible.length, total: relations.length }) }}
        </span>
      </div>
      <p v-if="!visible.length" class="tool-empty">{{ t('relationships.noMatches') }}</p>
      <div v-else class="relation-canvas">
        <article v-for="r in visible" :key="JSON.stringify([r.table, r.name])" :aria-label="r.name">
          <header class="constraint">
            <h2>
              <UiIcon name="link" :size="15" />
              {{ r.name }}
            </h2>
            <span v-if="r.onDelete">
              ON DELETE
              <b>{{ r.onDelete }}</b>
            </span>
            <span v-if="r.onUpdate">
              ON UPDATE
              <b>{{ r.onUpdate }}</b>
            </span>
          </header>
          <div class="mapping">
            <div class="endpoint">
              <small>{{ t('relationships.from') }}</small>
              <RouterLink :to="structure(r.table)">
                <UiIcon name="table" :size="17" />
                {{ r.table }}
              </RouterLink>
            </div>
            <div class="endpoint target">
              <small>{{ t('relationships.to') }}</small>
              <RouterLink :to="structure(r.refTable, r.refSchema)">
                <UiIcon name="table" :size="17" />
                {{ r.refSchema ? r.refSchema + '.' : '' }}{{ r.refTable }}
              </RouterLink>
            </div>
            <template v-for="(column, i) in r.columns" :key="i">
              <code>{{ column }}</code>
              <span class="connector" aria-hidden="true"><UiIcon name="chevron-right" :size="14" /></span>
              <code>{{ r.refColumns[i] }}</code>
            </template>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.relation-list {
  overflow: hidden;
}
.relation-filter {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--db-muted);
  max-width: 100%;
}
.relation-filter input {
  width: min(320px, 67vw);
}
.relation-canvas {
  padding: clamp(12px, 2vw, 24px);
  display: grid;
  gap: 18px;
  background-color: var(--db-sidebar);
  background-image: radial-gradient(var(--db-line) 1px, transparent 1px);
  background-size: 16px 16px;
}
.relation-canvas article {
  border: 1px solid var(--db-line);
  background: var(--db-panel);
  border-radius: 9px;
  min-width: 0;
  overflow: hidden;
}
.constraint {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 15px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--db-line);
}
.constraint h2 {
  margin-right: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  overflow-wrap: anywhere;
  min-width: 0;
  font: 600 13px var(--db-mono);
}
.constraint h2 svg {
  color: var(--db-accent);
}
.constraint span {
  color: var(--db-muted);
  font: 11px var(--db-mono);
}
.mapping {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(24px, 7%) minmax(0, 1fr);
  align-items: center;
  gap: 0 8px;
  padding: 13px 16px 16px;
}
.endpoint {
  align-self: stretch;
}
.endpoint.target {
  grid-column: 3;
}
.endpoint small {
  display: block;
  color: var(--db-muted);
  font-size: 11px;
}
.endpoint a {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--db-accent);
  font: 600 14px/1.45 var(--db-mono);
  text-decoration: none;
  overflow-wrap: anywhere;
  padding-block: 5px;
}
.endpoint a:hover {
  text-decoration: underline;
}
.mapping code {
  align-self: stretch;
  font: 12px/1.4 var(--db-mono);
  background: var(--db-sidebar);
  border-block-start: 1px solid var(--db-line);
  padding: 5px 8px;
  overflow-wrap: anywhere;
}
.connector {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  color: var(--db-accent);
}
.connector::before {
  content: "";
  flex: 1;
  border-top: 1px solid var(--db-accent);
  opacity: 0.55;
}
.connector svg {
  margin-left: -9px;
}
@media (max-width: 640px) {
  .mapping {
    padding: 9px;
    column-gap: 3px;
  }
  .endpoint a {
    font-size: 12px;
  }
  .constraint {
    padding: 9px;
  }
}
</style>

<template>
  <section class="db-view db-page">
    <CatalogHeader icon="db" :scope="t('sideNavigation.database')" :title="db"
      :engine="driver ? driver.name + ' ' + driver.version : undefined"
      :detail="catalog.schema ? t('workspace.schema', { name: catalog.schema }) : undefined" :stats="stats" />
    <ui-alert v-if="catalog.error || error" type="error">{{ catalog.error || error }}</ui-alert>
    <div class="db-page-head db-view__heading">
      <h2 class="db-section-title">{{ t('tree.tables') }}</h2>
      <div class="db-toolbar">
        <ui-button size="small" tag="a" :aria-label="t('relationships.title')" :title="t('relationships.title')" :href="catalog.path('/db/' + encodeURIComponent(db) + '/relationships')"
          @click.exact.prevent="router.push(catalog.path('/db/' + encodeURIComponent(db) + '/relationships'))">
          <UiIcon name="network" /><span>{{ t('relationships.title') }}</span>
        </ui-button>
        <ui-button size="small" type="primary" @click="newTable"><UiIcon name="plus" />{{ t('tree.newTable') }}</ui-button>
      </div>
    </div>
    <section class="db-panel catalog-grid" :aria-label="t('workspace.catalog')">
      <div class="db-view__controls db-toolbar">
        <label class="db-search"><UiIcon name="search" /><input v-model="filter" type="search"
          :aria-label="t('navigationWorkflows.filterTables')" :placeholder="t('workspace.findTable')" /></label>
        <label class="catalog-sort"><UiIcon name="sort" />
          <select v-model="sort" :aria-label="t('navigationWorkflows.sortTables')">
            <option value="name">{{ t('workspace.byName') }}</option><option value="rows">{{ t('workspace.byRows') }}</option>
            <option value="sizeBytes">{{ t('workspace.bySize') }}</option>
          </select>
        </label>
        <ui-button data-refresh-tables size="small" quaternary :aria-label="t('server.refresh')" :title="t('server.refresh')"
          :loading="catalog.loading" @click="load"><UiIcon name="refresh" /></ui-button>
      </div>
      <div class="db-view__selection">
        <div v-if="selected.length" class="db-view__bulk" role="group" :aria-label="t('databaseOps.selected', { count: selected.length })">
          <div class="db-view__selected-count">
            <strong>{{ t('databaseOps.selected', {count:selected.length}) }}</strong>
            <span v-if="hiddenSelected">{{ t('databaseOps.hiddenSelected', {count:hiddenSelected}) }}</span>
            <ui-button size="small" quaternary :disabled="busy" @click="selected = []">{{ t('databaseOps.clearSelection') }}</ui-button>
          </div>
          <div class="db-view__bulk-commands">
            <ui-popconfirm @positive-click="bulk('truncate')">
              <template #trigger><ui-button size="small" :disabled="busy">{{ t('navigationWorkflows.truncate') }}</ui-button></template>
              {{ t('navigationWorkflows.confirmTruncate', {names: db + ': ' + selected.join(', ')}) }}
            </ui-popconfirm>
            <ui-popconfirm @positive-click="bulk('drop')">
              <template #trigger><ui-button type="error" secondary size="small" :disabled="busy">{{ t('navigationWorkflows.drop') }}</ui-button></template>
              {{ t('navigationWorkflows.confirmDrop', {names: db + ': ' + selected.join(', ')}) }}
            </ui-popconfirm>
          </div>
        </div>
        <TableTransfer :db="db" :tables="selected" @changed="load" />
      </div>
      <div class="db-view__table">
        <table class="db-table">
          <thead><tr>
            <th class="check-cell"><input data-select-all type="checkbox" :checked="allSelected"
              :indeterminate="!allSelected && selectable.some(name => selected.includes(name))"
              :disabled="!selectable.length || busy || catalog.loading" :aria-label="t('navigationWorkflows.selectAll')"
              @change="selectAll(($event.target as HTMLInputElement).checked)" /></th>
            <th>{{ t('server.columns.name') }}</th><th v-if="hasDescription" class="catalog-description">{{ t('workspace.description') }}</th>
            <th class="number-cell">{{ t('workspace.rows') }}</th><th class="number-cell">{{ t('server.columns.size') }}</th>
            <th class="tools-cell"><span class="sr-actions">{{ t('navigationWorkflows.actions') }}</span></th>
          </tr></thead>
          <tbody><tr v-for="table in visibleTables" :key="table.name">
            <td class="check-cell"><input v-if="!table.isView" v-model="selected" type="checkbox" :value="table.name" :disabled="busy || catalog.loading" :aria-label="table.name" /></td>
            <td class="catalog-name-cell"><div class="catalog-name">
              <button class="table-name" @click="openTable(table.name)"><span class="db-object-icon"><UiIcon :name="table.isView ? 'eye' : 'table'" :size="15" /></span>{{ table.name }}</button>
              <span v-if="table.isView" class="db-tag">{{ t('navigationWorkflows.view') }}</span>
              <small v-else-if="table.engine">{{ table.engine }}</small>
            </div>
              <div class="catalog-mobile-meta">
                <span v-if="!table.isView">{{ t('workspace.rowEstimate', { n: formatter.format(table.rows) }) }}</span>
                <span>{{ formatBytes(table.sizeBytes) }}</span>
              </div>
            </td>
            <td v-if="hasDescription" class="catalog-description">{{ table.comment || '—' }}</td>
            <td class="number-cell">{{ table.isView ? '—' : '≈ ' + formatter.format(table.rows) }}</td>
            <td class="number-cell">{{ formatBytes(table.sizeBytes) }}</td>
            <td class="tools-cell"><div class="db-view__quick">
              <ui-button v-if="!table.isView" data-insert-row size="tiny" quaternary :title="t('navigationWorkflows.insert')" :aria-label="t('navigationWorkflows.insert')" @click="openTable(table.name, 'data', 'insert')"><UiIcon name="plus" :size="15" /></ui-button>
              <ui-dropdown :options="rowActions" @select="action => runTableAction(action, table.name)">
                <ui-button size="tiny" quaternary :title="t('workspace.tableActions')" :aria-label="t('workspace.tableActions') + ' · ' + table.name"><UiIcon name="more" /></ui-button>
              </ui-dropdown>
            </div></td>
          </tr>
          <tr v-if="!visibleTables.length && !catalog.loading && !catalog.error"><td :colspan="hasDescription ? 6 : 5" class="db-view__empty">
            <span v-if="!tables.length" data-empty-database>{{ t('databaseOps.emptyDatabase') }}</span>
            <template v-else><span data-no-matches>{{ t('databaseOps.noMatchingTables') }}</span>
              <ui-button data-clear-filter size="small" quaternary @click="filter = ''">{{ t('databaseOps.clearFilter') }}</ui-button></template>
          </td></tr></tbody>
        </table>
      </div>
    </section>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { UiAlert, UiButton, UiDropdown, UiPopconfirm } from '../ui'
import UiIcon from '../components/UiIcon.vue'
import CatalogHeader from '../components/CatalogHeader.vue'
import { useSessionStore } from '../stores/session'
import TableTransfer from '../components/TableTransfer.vue'
import { useCatalogStore } from '../stores/catalog'
import {api} from '../api/client'
import { formatBytes } from '../lib/format'
import {tableContextLink} from '../lib/tableQuery'
const { t, locale } = useI18n()
const session = useSessionStore()
const driver = computed(() => session.sessionInfo?.driver)
const route = useRoute(), router = useRouter(), catalog = useCatalogStore()
const db = computed(() => String(route.params.db ?? ''))
const selected = ref<string[]>([]), filter=ref(''),sort=ref('name'),busy=ref(false),error=ref('')
const tables = computed(() => catalog.tablesPath === catalog.path('/api/databases/' + encodeURIComponent(db.value) + '/tables') ? catalog.tables : [])
const formatter = computed(() => new Intl.NumberFormat(locale.value))
const hasDescription = computed(() => tables.value.some(table => table.comment))
const stats = computed(() => {
  const base = tables.value.filter(table => !table.isView)
  const number = (n: number) => formatter.value.format(n)
  const ready = !catalog.error && !catalog.loading
  return [
    { label: t('tree.tables'), icon: 'table', value: ready ? number(base.length) : '—' },
    { label: t('workspace.rows'), icon: 'rows', value: ready ? '≈ ' + number(base.reduce((sum, tab) => sum + tab.rows, 0)) : '—' },
    { label: t('workspace.storage'), icon: 'disk', value: ready ? formatBytes(base.reduce((sum, tab) => sum + tab.sizeBytes, 0)) : '—' },
  ]
})
const visibleTables = computed(() => {
  const query = filter.value.trim().toLocaleLowerCase()
  return tables.value.filter(table => [table.name, table.comment].some(text => text?.toLocaleLowerCase().includes(query)))
    .sort((a,b) => (sort.value === 'name' ? 0 : Number(b[sort.value as 'rows'|'sizeBytes']) - Number(a[sort.value as 'rows'|'sizeBytes'])) || a.name.localeCompare(b.name))
})
const rowActions = computed(() => [
  { key: 'structure', label: t('table.structure') },
  { key: 'console', label: t('navigationWorkflows.console') },
  { key: 'export', label: t('navigationWorkflows.export') },
  { key: 'operations', label: t('navigationWorkflows.operations') },
])
function runTableAction(action: string, name: string) {
  if (action === 'console' || action === 'export') {
    void router.push(tableContextLink(db.value, name, action, catalog.schema))
  } else {
    openTable(name, action)
  }
}
const selectable=computed(()=>visibleTables.value.filter(table=>!table.isView).map(table=>table.name))
const hiddenSelected = computed(() => selected.value.filter(name => !selectable.value.includes(name)).length)
const allSelected=computed(()=>selectable.value.length>0 && selectable.value.every(name=>selected.value.includes(name)))
function selectAll(checked:boolean):void{selected.value=checked?[...new Set([...selected.value,...selectable.value])]:selected.value.filter(name=>!selectable.value.includes(name))}
async function bulk(action:'truncate'|'drop'):Promise<void>{
 if(!selected.value.length||busy.value)return
 const tables=[...selected.value];busy.value=true;error.value=''
 try{await api('POST',catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables`),{action,tables});await load()}
 catch(err){error.value=err instanceof Error?err.message:String(err);await catalog.invalidate(db.value)}finally{busy.value=false}
}
async function load(): Promise<void> { selected.value=[]; await catalog.fetchTables(db.value) }
function openTable(name: string, view = 'data', action?: 'insert'): void {
  const query = new URLSearchParams({ view })
  if (action) query.set('action', action)
  void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent(name)}?${query}`))
}
function newTable(): void { openTable('_new','structure') }
watch([db, () => catalog.schema], () => {
  filter.value = ''
  error.value = ''
  void load()
}, { immediate: true })
</script>
<style scoped>
.catalog-grid { overflow: hidden; }
.db-view__controls { padding: 11px 14px; border-bottom: 1px solid var(--db-line); flex-wrap: nowrap; }
.catalog-sort { display: flex; align-items: center; gap: 5px; margin-left: auto; color: var(--db-muted); }
.catalog-sort select { background: var(--db-panel); color: var(--db-muted); border: 0; font-size: 12px; max-width: 145px; cursor: pointer; }
.db-view__table { max-width: 100%; overflow-x: auto; }
.db-view__table :deep(.db-table) { border-radius: 0; }
.check-cell { width: 32px; }
.catalog-name { display: flex; gap: 7px; align-items: center; flex-wrap: wrap; }
.catalog-name small, .catalog-description { font-size: 12px; color: var(--db-muted); }
.table-name { background: transparent; padding: 0; border: 0; display: inline-flex; align-items: center; gap: 9px; cursor: pointer; font: 550 13px var(--db-mono); color: var(--db-ink); text-align: left; overflow-wrap: anywhere; }
.table-name:hover { color: var(--db-accent); }
.db-view__table :deep(.number-cell) { text-align: right; white-space: nowrap; }
td.number-cell { font: 12px var(--db-mono); color: var(--db-muted); }
.tools-cell { width: 74px; }
.catalog-mobile-meta { display: none; }
.sr-actions { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.db-view__quick, .db-view__bulk { display: flex; align-items: center; gap: 5px; }
.db-view__selection:has(> *) {
  display: flex; align-items: center; flex-wrap: wrap; gap: 8px 14px;
  padding: 9px 14px; border-bottom: 1px solid var(--db-line); background: var(--db-tint);
}
.db-view__bulk, .db-view__selected-count, .db-view__bulk-commands { display: flex; align-items: center; flex-wrap: wrap; gap: 5px 8px; }
.db-view__bulk strong { font-size: 13px; }
.db-view__bulk span { color: var(--db-muted); font-size: 12px; }
.db-view__selection :deep(.table-transfer:has(form)) { flex-basis: 100%; }
.db-view__selection :deep(.transfer-form) { background: var(--db-panel); }
.db-view__quick { justify-content: end; }
.db-view__empty { text-align: center; padding: 22px; }
.db-view__empty button { margin-inline-start: 8px; }
@media(max-width:719px) {
  .db-view__controls { padding: 8px; gap: 4px; }
  .db-view__bulk { column-gap: 16px; }
  .catalog-sort select { max-width: 95px; }
  .db-view__heading .db-toolbar { gap: 5px; }
  .catalog-description, .db-view__table :deep(.number-cell) { display:none; }
  .catalog-name-cell { width: 100%; }
  .table-name { align-items: start; }
  .catalog-mobile-meta { display: flex; gap: 9px; flex-wrap: wrap; margin: 3px 0 0 37px; color: var(--db-muted); font-size: 11px; }
  .catalog-name .db-object-icon { width: 24px; height: 23px; }
  .db-view__table .check-cell { width: 20px; }
  .db-view__quick { gap: 1px; }
  .db-view__table :deep(.ui-button--tiny) { min-height: 30px; }
  .catalog-mobile-meta span + span::before { content: '·'; margin-right: 6px; }

  .db-view__table :deep(th),.db-view__table :deep(td) { padding: 7px 6px; }
  .catalog-name small { display: none; }
}
</style>

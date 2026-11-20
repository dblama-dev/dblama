<template>
  <section class="db-view">
    <n-breadcrumb><n-breadcrumb-item @click="router.push('/server')">{{ t('server.title') }}</n-breadcrumb-item><n-breadcrumb-item>{{ db }}</n-breadcrumb-item><n-breadcrumb-item v-if="catalog.schema">{{ catalog.schema }}</n-breadcrumb-item></n-breadcrumb>
    <header class="db-view__heading">
      <h2>{{ db }}</h2>
      <n-button data-database-actions size="small" :aria-expanded="showDatabaseActions" aria-controls="database-operations"
        @click="showDatabaseActions = !showDatabaseActions">{{ t('databaseOps.databaseActions') }} <span aria-hidden="true">{{ showDatabaseActions ? '▴' : '▾' }}</span></n-button>
    </header>
    <div v-show="showDatabaseActions" id="database-operations">
      <DatabaseActions :db="db" @changed="changed" @renamed="renamed" @dropped="dropped" />
    </div>
    <n-alert v-if="catalog.error || error" type="error">{{ catalog.error || error }}</n-alert>
    <n-space>
      <n-button size="small" type="primary" @click="newTable">{{ t('tree.newTable') }}</n-button>
      <n-button size="small" tag="a" :href="catalog.path(`/db/${encodeURIComponent(db)}/relationships`)" @click.exact.prevent="router.push(catalog.path(`/db/${encodeURIComponent(db)}/relationships`))">{{ t('relationships.title') }}</n-button>
      <n-button v-for="action in ['console','search','import','export']" :key="action" size="small" @click="router.push(catalog.path(`/db/${encodeURIComponent(db)}/${action}`))">{{ t(`navigationWorkflows.${action}`) }}</n-button>
    </n-space>
    <div class="db-view__controls">
      <n-input v-model:value="filter" :input-props="{ 'aria-label': t('navigationWorkflows.filterTables') }" :placeholder="t('navigationWorkflows.filterTables')" clearable />
      <n-select v-model:value="sort" :aria-label="t('navigationWorkflows.sortTables')" :options="sortOptions" />
      <n-button data-refresh-tables :loading="catalog.loading" @click="load">{{ t('server.refresh') }}</n-button>
    </div>
    <div class="db-view__table"><n-table size="small">
      <thead><tr><th><input data-select-all type="checkbox" :checked="allSelected" :indeterminate="!allSelected && selectable.some(name => selected.includes(name))" :disabled="!selectable.length || busy || catalog.loading" :aria-label="t('navigationWorkflows.selectAll')" @change="selectAll(($event.target as HTMLInputElement).checked)" /></th><th>{{ t('server.columns.name') }}</th><th>{{ t('navigationWorkflows.approximate') }}</th><th>{{ t('server.columns.size') }}</th><th>{{ t('navigationWorkflows.actions') }}</th></tr></thead>
      <tbody>
        <tr v-for="table in visibleTables" :key="table.name">
          <td><input v-if="!table.isView" v-model="selected" type="checkbox" :value="table.name" :disabled="busy || catalog.loading" :aria-label="table.name" /></td>
          <td><n-button text @click="openTable(table.name)">{{ table.name }}</n-button> <n-tag v-if="table.isView" size="small">{{ t('navigationWorkflows.view') }}</n-tag><small v-else>{{ table.engine }}</small></td>
          <td>{{ table.isView ? '—' : `≈ ${table.rows}` }}</td><td>{{ formatBytes(table.sizeBytes) }}</td>
          <td><div class="db-view__quick">
            <n-button size="tiny" @click="openTable(table.name)">{{ t('table.data') }}</n-button>
            <n-button size="tiny" @click="openTable(table.name, 'structure')">{{ t('table.structure') }}</n-button>
            <n-button v-if="!table.isView" data-insert-row size="tiny" @click="openTable(table.name, 'data', 'insert')">{{ t('navigationWorkflows.insert') }}</n-button>
            <n-button size="tiny" @click="router.push(tableContextLink(db,table.name,'console',catalog.schema))">SQL</n-button>
            <n-button size="tiny" @click="router.push(tableContextLink(db,table.name,'export',catalog.schema))">{{ t('navigationWorkflows.export') }}</n-button>
            <n-button size="tiny" @click="openTable(table.name,'operations')">{{ t('navigationWorkflows.operations') }}</n-button>
          </div></td>
        </tr>
        <tr v-if="!visibleTables.length && !catalog.loading && !catalog.error">
          <td colspan="5" class="db-view__empty">
            <span v-if="!tables.length" data-empty-database>{{ t('databaseOps.emptyDatabase') }}</span>
            <template v-else><span data-no-matches>{{ t('databaseOps.noMatchingTables') }}</span>
              <n-button data-clear-filter size="small" text @click="filter = ''">{{ t('databaseOps.clearFilter') }}</n-button>
            </template>
          </td>
        </tr>
      </tbody>
    </n-table></div>
    <div v-if="selected.length" class="db-view__bulk">
      <strong>{{ t('databaseOps.selected', {count:selected.length}) }}</strong>
      <span v-if="hiddenSelected">{{ t('databaseOps.hiddenSelected', {count:hiddenSelected}) }}</span>
      <n-button size="small" :disabled="busy" @click="selected = []">{{ t('databaseOps.clearSelection') }}</n-button>
      <n-popconfirm @positive-click="bulk('truncate')">
        <template #trigger><n-button size="small" :disabled="busy">{{ t('navigationWorkflows.truncate') }}</n-button></template>
        {{ t('navigationWorkflows.confirmTruncate', {names: db + ': ' + selected.join(', ')}) }}
      </n-popconfirm>
      <n-popconfirm @positive-click="bulk('drop')">
        <template #trigger><n-button type="error" secondary size="small" :disabled="busy">{{ t('navigationWorkflows.drop') }}</n-button></template>
        {{ t('navigationWorkflows.confirmDrop', {names: db + ': ' + selected.join(', ')}) }}
      </n-popconfirm>
    </div>
    <TableTransfer :db="db" :tables="selected" @changed="load" />
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NBreadcrumb,NBreadcrumbItem,NButton, NInput,NPopconfirm,NSelect,NSpace, NTable,NTag } from 'naive-ui'
import DatabaseActions from '../components/DatabaseActions.vue'
import TableTransfer from '../components/TableTransfer.vue'
import { useCatalogStore } from '../stores/catalog'
import {api} from '../api/client'
import { formatBytes } from '../lib/format'
import {tableContextLink} from '../lib/tableQuery'
const { t } = useI18n()
const route = useRoute(), router = useRouter(), catalog = useCatalogStore()
const db = computed(() => String(route.params.db ?? ''))
const selected = ref<string[]>([]), filter=ref(''),sort=ref('name'),busy=ref(false),error=ref('')
const showDatabaseActions = ref(false)
const tables = computed(() => catalog.tablesPath === catalog.path('/api/databases/' + encodeURIComponent(db.value) + '/tables') ? catalog.tables : [])
const sortOptions=computed(()=>[{label:t('server.columns.name'),value:'name'},{label:t('navigationWorkflows.approximate'),value:'rows'},{label:t('server.columns.size'),value:'sizeBytes'}])
const visibleTables=computed(()=>tables.value.filter(table=>table.name.toLowerCase().includes(filter.value.trim().toLowerCase())).sort((a,b)=>sort.value==='name'?a.name.localeCompare(b.name):Number(b[sort.value as 'rows'|'sizeBytes'])-Number(a[sort.value as 'rows'|'sizeBytes'])))
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
async function changed(): Promise<void> { catalog.databaseRevision++; await load() }
function renamed(name:string): void { void router.replace(catalog.path(`/db/${encodeURIComponent(name)}`)) }
function dropped(): void { void router.replace('/server') }
function openTable(name: string, view = 'data', action?: 'insert'): void {
  const query = new URLSearchParams({ view })
  if (action) query.set('action', action)
  void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent(name)}?${query}`))
}
function newTable(): void { openTable('_new','structure') }
watch([db, () => catalog.schema], () => {
  filter.value = ''
  error.value = ''
  showDatabaseActions.value = false
  void load()
}, { immediate: true })
</script>
<style scoped>
.db-view { display:flex; flex-direction:column; gap:16px; }
.db-view__heading, .db-view__bulk { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.db-view__heading { justify-content: space-between; }
.db-view__heading h2 { margin: 0; overflow-wrap: anywhere; }
.db-view__heading span[aria-hidden] { margin-inline-start: 5px; }
.db-view__bulk { padding-top: 6px; }
.db-view__empty { text-align: center; }
.db-view__empty button { margin-inline-start: 10px; }
.db-view__controls{display:flex;gap:8px;flex-wrap:wrap;}.db-view__controls .n-input{max-width:400px;}.db-view__controls .n-select{width:200px;}.db-view__table{max-width:100%;overflow:auto;}.db-view__quick{display:flex;gap:5px;flex-wrap:wrap;}small{opacity:.6;}
</style>

<template>
  <div class="object-tree">
    <div class="object-tree__scope">
      <label :for="databaseID">{{ t('sideNavigation.database') }}</label>
      <ui-select size="small" filterable data-testid="database-select"
        :input-props="{ id: databaseID, 'aria-label': t('sideNavigation.database') }"
        :value="db" :options="dbOptions" :loading="dbsLoading" :placeholder="t('tree.selectDb')" @update:value="selectDb" />
      <template v-if="session.sessionInfo?.caps.schemas">
        <label :for="schemaID">{{ t('sideNavigation.schema') }}</label>
        <ui-select data-testid="schema-select" filterable size="small"
          :input-props="{ id: schemaID, 'aria-label': t('sideNavigation.schema') }"
          :value="catalog.schema" :options="schemas.map(name => ({ label: name, value: name }))"
          :placeholder="t('tree.schemas')" @update:value="selectSchema" />
      </template>
    </div>
    <ui-alert v-if="schemaError" type="error">{{ schemaError }}</ui-alert>
    <ui-modal v-model:show="sequenceDialog" :title="t(sequenceEditName ? 'tree.editSequence' : 'tree.newSequence')" style="width: min(380px, calc(100vw - 24px))">
      <div class="sequence-form"><label>{{ t('tree.sequenceName') }}
      <ui-input v-model:value="sequenceName" :disabled="sequenceEditName !== null" :placeholder="t('tree.sequenceName')" data-testid="sequence-name" /></label>
      <label>{{ t('tree.sequenceIncrement') }}
      <ui-input-number v-model:value="sequenceIncrement" :min="-Number.MAX_SAFE_INTEGER" :max="Number.MAX_SAFE_INTEGER" :precision="0" /></label>
      <label v-if="sequenceEditName">{{ t('tree.sequenceRestart') }}<ui-input-number v-model:value="sequenceRestart" :min="-Number.MAX_SAFE_INTEGER" :max="Number.MAX_SAFE_INTEGER" :precision="0" clearable /></label>
      <ui-button :disabled="!sequenceName.trim() || sequenceIncrement === 0 || sequenceBusy" :loading="sequenceBusy"
        data-testid="sequence-create" @click="addSequence">{{ t(sequenceEditName ? 'row.save' : 'tree.newSequence') }}</ui-button>
      </div>
    </ui-modal>
    <div class="object-tree__navigation">
      <router-link v-if="db" class="section-link" :to="catalog.path(`/db/${encodeURIComponent(db)}`)">
        <UiIcon name="overview" :size="16" />{{ t('sideNavigation.overview') }}
      </router-link>
      <nav v-if="db" class="object-tree__tools" :aria-label="t('sideNavigation.tools')">
        <router-link v-for="(symbol, action) in databaseTools" :key="action" class="section-link"
          :to="catalog.path(`/db/${encodeURIComponent(db)}/${action}`)">
          <UiIcon :name="symbol" :size="16" />{{ t(`navigationWorkflows.${action}`) }}
        </router-link>
      </nav>
      <slot name="tools" :db="db" />
    </div>
    <div v-if="favoriteTables.length || recentTables.length" class="object-tree__bookmarks">
      <details v-if="favoriteTables.length" class="object-tree__saved" data-favorite-tables>
        <summary><UiIcon name="star" :size="15" />{{ t('navigationWorkflows.favorites') }}<span>{{ favoriteTables.length }}</span></summary>
        <button v-for="item in favoriteTables" :key="JSON.stringify(item)" type="button" @click="openSaved(item)">
          <span>{{ item.table }}</span><small>{{ item.db }}<template v-if="item.schema"> / {{ item.schema }}</template></small>
        </button>
      </details>
      <details v-if="recentTables.length" class="object-tree__saved">
        <summary><UiIcon name="history" :size="15" />{{ t('navigationWorkflows.recent') }}<span>{{ recentTables.length }}</span></summary>
        <button v-for="item in recentTables" :key="JSON.stringify(item)" type="button" @click="openSaved(item)">
          <span>{{ item.table }}</span><small>{{ item.db }}<template v-if="item.schema"> / {{ item.schema }}</template></small>
        </button>
      </details>
    </div>
    <div v-if="db" class="object-tree__explorer">
      <label :for="searchID">{{ t('shell.explorer') }}</label>
      <ui-input v-model:value="search" size="small" clearable :placeholder="t('shell.findObject')"
        :input-props="{ id: searchID, 'aria-label': t('sideNavigation.filter') }">
        <template #prefix><UiIcon name="search" :size="14" /></template>
      </ui-input>
    </div>
    <ui-alert v-if="dbsError" type="error" :title="t('tree.selectDb')">{{ dbsError }}</ui-alert>
    <ui-alert v-else-if="catalog.error" type="error" :title="t('tree.loadFailed')">{{ catalog.error }}</ui-alert>
    <div class="object-tree__list"><ui-tree
      v-if="!dbsError && !catalog.error"
      :data="treeData"
      :selected-keys="selectedKeys"
      :expanded-keys="expandedKeys"
      :disabled="catalog.loading"
      @update:expanded-keys="onExpand"
      @update:selected-keys="onSelect"
    /></div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { UiAlert, UiButton, UiInput, UiSelect, UiTree, UiModal, UiInputNumber, UiPopconfirm, type TreeOption } from '../ui'

import { fetchSchemas, fetchSequences, createSequence, dropSequence, alterSequence, withSchema } from '../lib/schema'
import type { SequenceInfo } from '../api/types'
import { api } from '../api/client'
import UiIcon from './UiIcon.vue'
import { databaseTools } from '../lib/workspaceNavigation'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import { readNavigationPreferences,writeNavigationPreferences,rememberTable,toggleFavorite,type TableLocation } from '../lib/navigationPreferences'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const catalog = useCatalogStore()
const session = useSessionStore()

const db = ref<string | null>(null)
const databaseID = useId()
const schemaID = useId()
const searchID = useId()
const routeDatabase = computed(() => typeof route.params.db === 'string' ? route.params.db : null)
const dbs = ref<string[]>([])
const dbsLoading = ref(false)
const dbsError = ref('')
const schemas = ref<string[]>([])
const sequences = ref<SequenceInfo[]>([])
const schemaError = ref('')
const sequenceDialog = ref(false)
const sequenceName = ref('')
const sequenceEditName = ref<string | null>(null)
const sequenceRestart = ref<number | null>(null)
const sequenceIncrement = ref<number | null>(1)
const sequenceBusy = ref(false)
let schemaLoadID = 0
async function loadSequences(): Promise<void> {
  const id = ++schemaLoadID
  if (!session.sessionInfo?.caps.sequences) return
  const schema = catalog.schema ?? 'public'
  const next = await fetchSequences(schema)
  if (id === schemaLoadID) sequences.value = next
}
async function selectSchema(schema: string): Promise<void> {
  if (!db.value) return
  const database=db.value
  // Navigate before changing shared scope so dirty editors may cancel.
  const failure=await router.push(withSchema(`/db/${encodeURIComponent(database)}`,schema,true))
  if(failure)return
  schemaError.value = ''
  if(catalog.schema!==schema)await catalog.selectSchema(database,schema)
}
async function addSequence(): Promise<void> {
  sequenceBusy.value = true
  schemaError.value = ''
  try {
    if (sequenceEditName.value) {
      await alterSequence(sequenceEditName.value, {increment: sequenceIncrement.value ?? 1, ...(sequenceRestart.value === null ? {} : {restartWith: sequenceRestart.value})}, catalog.schema ?? 'public')
    } else await createSequence(sequenceName.value, sequenceIncrement.value ?? 1, catalog.schema ?? 'public')
    sequenceDialog.value = false
    sequenceName.value = ''
    await loadSequences()
  } catch (err) { schemaError.value = String(err) }
  finally { sequenceBusy.value = false }
}
async function removeSequence(name: string): Promise<void> {
  try {
    await dropSequence(name, catalog.schema ?? 'public')
    await loadSequences()
  } catch (err) { schemaError.value = String(err) }
}

const search = ref('')
const selectedKeys = computed(() => typeof route.params.t === 'string' ? [`table:${route.params.t}`] : [])
const preferences=ref(readNavigationPreferences())
const scope=computed(()=>JSON.stringify(session.sessionInfo?{driver:session.sessionInfo.driver.flavor,host:session.sessionInfo.driver.host,port:session.sessionInfo.driver.port ?? '',user:session.sessionInfo.driver.user}:{}))
const recentTables=computed(()=>preferences.value.recent.filter(item=>item.scope===scope.value))
const favoriteTables=computed(()=>preferences.value.favorites.filter(item=>item.scope===scope.value))
function location(table:string):TableLocation{return {scope:scope.value,db:db.value??'',table,schema:catalog.schemaAware?catalog.schema:null}}
function persistPreferences():void{writeNavigationPreferences(undefined,{...readNavigationPreferences(),recent:preferences.value.recent,favorites:preferences.value.favorites})}
function favorite(table:string):void{preferences.value=toggleFavorite(preferences.value,location(table));persistPreferences()}
function isFavorite(table:string):boolean{return favoriteTables.value.some(item=>item.db===db.value&&item.schema===location(table).schema&&item.table===table)}
function openSaved(item:TableLocation):void{
 const query=new URLSearchParams();if(item.schema)query.set('schema',item.schema)
 void router.push(`/db/${encodeURIComponent(item.db)}/table/${encodeURIComponent(item.table)}${query.size?'?'+query:''}`)
}
watch([()=>route.params.db,()=>route.params.t,()=>catalog.schema,scope],()=>{
 if(typeof route.params.db!=='string'||typeof route.params.t!=='string'||route.params.t==='_new')return
 preferences.value=rememberTable(preferences.value,{...location(route.params.t),db:route.params.db});persistPreferences()
},{immediate:true})
const expandedKeys = ref<string[]>([])

const dbOptions = computed(() => dbs.value.map((name) => ({ label: name, value: name })))

function loadTables(database: string, refresh = false): Promise<void> {
  // Only the table header shares this initial request. Other sidebar mounts refresh the catalog.
  if (!refresh && route.name === 'db-table' && routeDatabase.value === database && route.params.t !== '_new') {
    return catalog.ensureTables(database)
  }
  return catalog.fetchTables(database)
}

/** Keep the sidebar and shared catalog on the open route's database.
 * Outside a database route, retain the selection or use the login database. */
async function loadDatabases(refreshTables = false): Promise<void> {
  catalog.schemaAware = session.sessionInfo?.caps.schemas ?? false
  if (catalog.schemaAware) {
    catalog.schema ??= 'public'
    try {
      schemas.value = await fetchSchemas()
      await loadSequences()
    } catch (err) { schemaError.value = String(err) }
  } else { catalog.schema = null }
  dbsLoading.value = true
  dbsError.value = ''
  try {
    const payload = await api<{ databases: { name: string }[] }>('GET', '/api/databases')
    dbs.value = (payload.databases ?? []).map((d) => d.name)
  } catch (e) {
    dbsError.value = e instanceof Error ? e.message : String(e)
  } finally {
    dbsLoading.value = false
  }
  const initial = db.value ?? session.sessionInfo?.database
  // A rename may update the route before this list response catches up.
  db.value = routeDatabase.value ?? (initial && dbs.value.includes(initial) ? initial : (dbs.value[0] ?? null))
  expandedKeys.value = defaultExpandedKeys(db.value)
  if (db.value !== null) await loadTables(db.value, refreshTables)
}
onMounted(loadDatabases)
watch(() => catalog.databaseRevision, () => loadDatabases(true))

watch(routeDatabase, async next => {
  if (!next || next === db.value) return
  db.value = next
  expandedKeys.value = defaultExpandedKeys(next)
  await loadTables(next)
})

watch(() => catalog.schema, async () => {
  if (!db.value || !catalog.schemaAware) return
  schemaError.value = ''
  sequences.value = []
  await loadTables(db.value)
  try { await loadSequences() } catch (err) { schemaError.value = String(err) }
})

async function selectDb(next: string): Promise<void> {
  const failure=await router.push(catalog.path(`/db/${encodeURIComponent(next)}`))
  if(failure)return
  if(db.value===next)return
  db.value=next
  expandedKeys.value=defaultExpandedKeys(next)
  await loadTables(next)
}

/** Keep the current database's table and view groups expanded on first load. */
function defaultExpandedKeys(database: string | null): string[] {
  return database === null ? [] : ['group:tables', 'group:views', 'group:sequences']
}

function onExpand(keys: string[]): void {
  expandedKeys.value = keys
}

const treeData = computed<TreeOption[]>(() => {
  if (db.value === null) return []
  const needle = search.value.trim().toLowerCase()
  const matches = (name: string): boolean => name.toLowerCase().includes(needle)
  const leaves = (views: boolean): TreeOption[] => catalog.tables
    .filter(table => table.isView === views && matches(table.name))
    .map(({name}) => ({
      key: 'table:' + name, label: name, isLeaf: true,
      prefix: () => h(UiIcon, {name: views ? 'eye' : 'table', size: 14}),
      suffix: () => h(UiButton, {
        size: 'tiny', quaternary: true, class: ['tree-star', {active: isFavorite(name)}],
        'aria-label': t('navigationWorkflows.favorite') + ': ' + name, 'aria-pressed': isFavorite(name),
        title: t('navigationWorkflows.favorite') + ': ' + name,
        onClick: (event: MouseEvent) => { event.stopPropagation(); favorite(name) },
      }, {default: () => h(UiIcon, { name: 'star', size: 13 })}),
    }))
  const children: TreeOption[] = [
    {
      key: 'group:tables',
      label: t('tree.tables'),
      children: leaves(false),
      // The "+" opens the structure editor on the _new placeholder to
      // create a table; stopPropagation keeps the click from selecting
      // or expanding the group node.
      suffix: () =>
        h(UiButton, {
          size: 'tiny',
          quaternary: true,
          title: t('tree.newTable'),
          'data-testid': 'tree-add-table',
          onClick: (e: MouseEvent) => {
            e.stopPropagation()
            void createTable()
          },
        }, { default: () => h(UiIcon, {name:'plus', size:14}) }),
    },
  ]
  const views = leaves(true)
  // The Views group appears only when the database actually has views.
  if (views.length > 0) {
    children.push({ key: 'group:views', label: t('tree.views'), children: views })
  }
  if (session.sessionInfo?.caps.sequences) {
    children.push({ key: 'group:sequences', label: t('tree.sequences'),
      suffix: () => h(UiButton, { size: 'tiny', quaternary: true, 'data-testid': 'tree-add-sequence', title: t('tree.newSequence'), onClick: (e: MouseEvent) => { e.stopPropagation(); sequenceEditName.value = null; sequenceName.value = ''; sequenceIncrement.value = 1; sequenceRestart.value = null; sequenceDialog.value = true } }, { default: () => h(UiIcon, {name:'plus', size:14}) }),
      children: sequences.value.filter(seq => matches(seq.name)).map(seq => ({ key: `sequence:${seq.name}`, label: seq.name, isLeaf: true,
        suffix: () => h('span', [h(UiButton, {size:'tiny',quaternary:true,title:t('tree.editSequence'),onClick:(e:MouseEvent)=>{e.stopPropagation();sequenceEditName.value=seq.name;sequenceName.value=seq.name;sequenceIncrement.value=seq.increment;sequenceRestart.value=null;sequenceDialog.value=true}}, {default:()=>h(UiIcon,{name:'edit',size:13})}), h(UiPopconfirm, { onPositiveClick: () => removeSequence(seq.name) }, {
          default: () => t('tree.dropSequenceConfirm', { name: seq.name }),
          trigger: () => h(UiButton, { size: 'tiny', quaternary: true, title: t('tree.dropSequence'), onClick: (e: MouseEvent) => e.stopPropagation() }, { default: () => h(UiIcon,{name:'x',size:13}) }),
        })]),
      })),
    })
  }
  return children
})

function onSelect(keys: string[]): void {
  const key = keys[keys.length - 1]
  if (key === undefined || db.value === null) return
  // Identifiers may contain %, ? or #; vue-router decodes params on read.
  if (!key.startsWith('table:')) return
  const table = key.slice('table:'.length)
  void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent(table)}`))
}

/** Opens the create-table structure editor on the _new placeholder. */
function createTable(): void {
  if (db.value === null) return
  void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent('_new')}?view=structure`))
}
</script>

<style scoped>
.object-tree { flex: 1; min-height: 0; overflow: auto; scrollbar-width: thin; scrollbar-color: var(--db-line) transparent; padding: 7px 11px 16px; }
.object-tree__scope, .object-tree__explorer, .sequence-form, .sequence-form label { display: grid; gap: 5px; }
.object-tree__scope label, .object-tree__explorer label { margin: 1px 1px 0; font-size: 11px; font-weight: 600; color: var(--db-muted); }
.object-tree__scope label:not(:first-child) { margin-top: 3px; }
.object-tree__navigation { display: grid; gap: 1px; margin: 12px 0 9px; }
.object-tree__tools { display: grid; gap: 1px; }
.section-link, .object-tree__saved summary { display: flex; align-items: center; gap: 9px; padding: 6px 9px; box-sizing: border-box; min-height: 30px; line-height: 1.3; border-radius: 6px; color: var(--db-ink); font-size: 13px; text-decoration: none; }
.section-link svg, .object-tree__saved svg, :deep(.ui-input__prefix), :deep(.ui-tree-node-content__prefix) { color: var(--db-muted); }
.section-link:hover, .section-link.router-link-exact-active, .object-tree__saved summary:hover { background: var(--db-tint); color: var(--db-accent); }
.section-link.router-link-exact-active { font-weight: 600; }
.section-link.router-link-exact-active svg { color: var(--db-accent); }
.object-tree__bookmarks { border-top: 1px solid var(--db-line); padding: 6px 0; }
.object-tree__saved summary { list-style: none; cursor: pointer; }
.object-tree__saved summary::-webkit-details-marker { display: none; }
.object-tree__saved summary span { margin-left: auto; font-size: 10px; color: var(--db-muted); }
.object-tree__saved button { margin: 2px 0; display: flex; flex-direction: column; align-items: flex-start; width: 100%; padding: 5px 8px 5px 29px; overflow-wrap: anywhere; text-align: start; font: 12px/1.4 var(--db-mono); color: var(--db-ink); border: none; border-radius: 6px; background: none; cursor: pointer; }
.object-tree__saved button small { color: var(--db-muted); font-size: 10px; }
.object-tree__saved button:hover { background: var(--db-tint); color: var(--db-accent); }
.object-tree__explorer { border-top: 1px solid var(--db-line); padding: 10px 0 7px; }
.object-tree__list { min-width: 0; }
.object-tree__list :deep(.ui-tree-node) { border-radius: 5px; }
.object-tree__list :deep(.ui-tree-node-content) { min-height: 28px; min-width: 0; align-items: center; }
.object-tree__list :deep(.ui-tree-node-content__text) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.object-tree__list :deep(.ui-tree-node-content__prefix) { flex-shrink: 0; }
.object-tree__list :deep(.tree-star) { opacity: .58; }
.object-tree__list :deep(.tree-star.active), .object-tree__list :deep(.tree-star:is(:hover, :focus-visible)) { opacity: 1; color: var(--db-accent); }
.object-tree__list :deep(.tree-star.active svg) { fill: var(--db-tint); }
@media (max-width:719px), (pointer:coarse) {
  .object-tree__list :deep(.ui-tree-node) { min-height:36px; }
  .object-tree__list :deep(.tree-star) { min-width:34px; min-height:34px; }
}
.sequence-form { gap: 12px; }
.sequence-form :deep(.ui-button) { justify-self: end; }
</style>

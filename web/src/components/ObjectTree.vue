<template>
  <div class="object-tree">
    <div class="object-tree__scope">
      <label :for="databaseID">{{ t('sideNavigation.database') }}</label>
      <router-link v-if="db" :to="catalog.path(`/db/${encodeURIComponent(db)}`)">{{ t('sideNavigation.overview') }}</router-link>
    </div>
    <n-select
      size="small"
      filterable data-testid="database-select" :input-props="{ id: databaseID, 'aria-label': t('sideNavigation.database') }"
      :value="db"
      :options="dbOptions"
      :loading="dbsLoading"
      :placeholder="t('tree.selectDb')"
      @update:value="selectDb"
    />
    <label v-if="session.sessionInfo?.caps.schemas" :for="schemaID" class="object-tree__scope">{{ t('sideNavigation.schema') }}</label>
    <n-select v-if="session.sessionInfo?.caps.schemas" data-testid="schema-select" filterable
      :input-props="{ id: schemaID, 'aria-label': t('sideNavigation.schema') }"
      :value="catalog.schema" :options="schemas.map(name => ({ label: name, value: name }))"
      :placeholder="t('tree.schemas')" size="small" @update:value="selectSchema" />
    <n-alert v-if="schemaError" type="error">{{ schemaError }}</n-alert>
    <n-modal v-model:show="sequenceDialog" preset="card" :title="t(sequenceEditName ? 'tree.editSequence' : 'tree.newSequence')" style="width: 380px">
      <n-input v-model:value="sequenceName" :disabled="sequenceEditName !== null" :placeholder="t('tree.sequenceName')" data-testid="sequence-name" />
      <label>{{ t('tree.sequenceIncrement') }}</label>
      <n-input-number v-model:value="sequenceIncrement" :min="-Number.MAX_SAFE_INTEGER" :max="Number.MAX_SAFE_INTEGER" :precision="0" />
      <template v-if="sequenceEditName"><label>{{ t('tree.sequenceRestart') }}</label><n-input-number v-model:value="sequenceRestart" :min="-Number.MAX_SAFE_INTEGER" :max="Number.MAX_SAFE_INTEGER" :precision="0" clearable /></template>
      <n-button :disabled="!sequenceName.trim() || sequenceIncrement === 0 || sequenceBusy" :loading="sequenceBusy"
        data-testid="sequence-create" @click="addSequence">{{ t(sequenceEditName ? 'row.save' : 'tree.newSequence') }}</n-button>
    </n-modal>
    <nav v-if="db" class="object-tree__tools" :aria-label="t('sideNavigation.tools')">
      <router-link v-for="action in ['console', 'search', 'import', 'export']" :key="action" :to="catalog.path(`/db/${encodeURIComponent(db)}/${action}`)">
        {{ t(`navigationWorkflows.${action}`) }}
      </router-link>
    </nav>
    <n-input
      v-model:value="search"
      size="small"
      clearable
      :placeholder="t('sideNavigation.filter')" :input-props="{ 'aria-label': t('sideNavigation.filter') }"
    />
    <n-alert v-if="dbsError" type="error" :title="t('tree.selectDb')">
      {{ dbsError }}
    </n-alert>
    <n-alert v-else-if="catalog.error" type="error" :title="t('tree.loadFailed')">
      {{ catalog.error }}
    </n-alert>
    <div class="object-tree__list"><n-tree
      v-if="!dbsError && !catalog.error"
      block-line
      :data="treeData"
      :selected-keys="selectedKeys"
      :expanded-keys="expandedKeys"
      :disabled="catalog.loading"
      @update:expanded-keys="onExpand"
      @update:selected-keys="onSelect"
    /></div>
    <footer class="object-tree__footer">
      <details v-if="favoriteTables.length" class="object-tree__saved" data-favorite-tables>
        <summary>{{ t('navigationWorkflows.favorites') }} ({{ favoriteTables.length }})</summary>
        <n-button v-for="item in favoriteTables" :key="JSON.stringify(item)" text size="small" @click="openSaved(item)">{{ item.db }} / {{ item.table }}<span v-if="item.schema"> ({{ item.schema }})</span></n-button>
      </details>
      <details v-if="recentTables.length" class="object-tree__saved">
        <summary>{{ t('navigationWorkflows.recent') }} ({{ recentTables.length }})</summary>
        <n-button v-for="item in recentTables" :key="JSON.stringify(item)" text size="small" @click="openSaved(item)">{{ item.db }} / {{ item.table }}<span v-if="item.schema"> ({{ item.schema }})</span></n-button>
      </details>
      <slot name="footer" :db="db" />
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NInput, NSelect, NTree, NModal, NInputNumber, NPopconfirm, type TreeOption } from 'naive-ui'

import { fetchSchemas, fetchSequences, createSequence, dropSequence, alterSequence, withSchema } from '../lib/schema'
import type { SequenceInfo } from '../api/types'
import { api } from '../api/client'
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

/** Keep the sidebar and shared catalog on the open route's database.
 * Outside a database route, retain the selection or use the login database. */
async function loadDatabases(): Promise<void> {
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
  if (db.value !== null) await catalog.fetchTables(db.value)
}
onMounted(loadDatabases)
watch(() => catalog.databaseRevision, loadDatabases)

watch(routeDatabase, async next => {
  if (!next || next === db.value) return
  db.value = next
  expandedKeys.value = defaultExpandedKeys(next)
  await catalog.fetchTables(next)
})

watch(() => catalog.schema, async () => {
  if (!db.value || !catalog.schemaAware) return
  schemaError.value = ''
  sequences.value = []
  await catalog.fetchTables(db.value)
  try { await loadSequences() } catch (err) { schemaError.value = String(err) }
})

async function selectDb(next: string): Promise<void> {
  const failure=await router.push(catalog.path(`/db/${encodeURIComponent(next)}`))
  if(failure)return
  if(db.value===next)return
  db.value=next
  expandedKeys.value=defaultExpandedKeys(next)
  await catalog.fetchTables(next)
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
  const tables = catalog.tables
    .filter((table) => !table.isView && matches(table.name))
    .map((table) => ({ key: `table:${table.name}`, label: table.name, isLeaf: true, suffix:()=>h(NButton,{size:'tiny',quaternary:true,'aria-label':`${t('navigationWorkflows.favorite')}: ${table.name}`,onClick:(event:MouseEvent)=>{event.stopPropagation();favorite(table.name)}},{default:()=>isFavorite(table.name)?'★':'☆'}) }))
  const children: TreeOption[] = [
    {
      key: 'group:tables',
      label: t('tree.tables'),
      children: tables,
      // The "+" opens the structure editor on the _new placeholder to
      // create a table; stopPropagation keeps the click from selecting
      // or expanding the group node.
      suffix: () =>
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          title: t('tree.newTable'),
          'data-testid': 'tree-add-table',
          onClick: (e: MouseEvent) => {
            e.stopPropagation()
            void createTable()
          },
        }, { default: () => '+' }),
    },
  ]
  const views = catalog.tables
    .filter((table) => table.isView && matches(table.name))
    .map((table) => ({ key: `table:${table.name}`, label: table.name, isLeaf: true, suffix:()=>h(NButton,{size:'tiny',quaternary:true,'aria-label':`${t('navigationWorkflows.favorite')}: ${table.name}`,onClick:(event:MouseEvent)=>{event.stopPropagation();favorite(table.name)}},{default:()=>isFavorite(table.name)?'★':'☆'}) }))
  // The Views group appears only when the database actually has views.
  if (views.length > 0) {
    children.push({ key: 'group:views', label: t('tree.views'), children: views })
  }
  if (session.sessionInfo?.caps.sequences) {
    children.push({ key: 'group:sequences', label: t('tree.sequences'),
      suffix: () => h(NButton, { size: 'tiny', quaternary: true, 'data-testid': 'tree-add-sequence', title: t('tree.newSequence'), onClick: (e: MouseEvent) => { e.stopPropagation(); sequenceEditName.value = null; sequenceName.value = ''; sequenceIncrement.value = 1; sequenceRestart.value = null; sequenceDialog.value = true } }, { default: () => '+' }),
      children: sequences.value.filter(seq => matches(seq.name)).map(seq => ({ key: `sequence:${seq.name}`, label: seq.name, isLeaf: true,
        suffix: () => h('span', [h(NButton, {size:'tiny',quaternary:true,title:t('tree.editSequence'),onClick:(e:MouseEvent)=>{e.stopPropagation();sequenceEditName.value=seq.name;sequenceName.value=seq.name;sequenceIncrement.value=seq.increment;sequenceRestart.value=null;sequenceDialog.value=true}}, {default:()=> '↺'}), h(NPopconfirm, { onPositiveClick: () => removeSequence(seq.name) }, {
          default: () => t('tree.dropSequenceConfirm', { name: seq.name }),
          trigger: () => h(NButton, { size: 'tiny', quaternary: true, title: t('tree.dropSequence'), onClick: (e: MouseEvent) => e.stopPropagation() }, { default: () => '×' }),
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
.object-tree {
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  overflow: auto;
  gap: 6px;
  padding: 8px 12px;
}
.object-tree > * { flex-shrink: 0; }
.object-tree__scope { display: flex; align-items: baseline; justify-content: space-between; font-size: 12px; }
.object-tree__scope label { font-weight: 600; }
.object-tree__scope a, .object-tree__tools a { color: inherit; }
.object-tree__tools { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 3px; padding: 2px 0 4px; }
.object-tree__tools a { padding: 3px 7px; border-radius: 3px; background: #8881; font-size: 13px; text-decoration: none; }
.object-tree__tools a:hover, .object-tree__tools a:focus-visible, .object-tree__tools .router-link-exact-active { background: #8883; text-decoration: underline; }
.object-tree__list { flex: 1 0 80px; min-height: 80px; overflow: auto; }
.object-tree__footer { border-top: 1px solid #8884; padding-top: 5px; max-height: 38vh; overflow: auto; }
.object-tree__saved summary { cursor: pointer; padding: 4px 0; }
.object-tree__saved .n-button { display: flex; max-width: 100%; justify-content: flex-start; text-align: start; padding: 3px 8px; white-space: normal; overflow-wrap: anywhere; }
</style>

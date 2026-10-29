<template>
  <section class="console">
    <h2>{{ t('sqlWorkflows.context', { db }) }}</h2>
    <div class="console__toolbar">
      <n-button size="small" type="primary" :loading="c.executing.value" @click="runAll">{{ t('console.executeAll') }}</n-button>
      <n-button size="small" :disabled="c.executing.value" @click="runSelection">{{ t('console.executeSelection') }}</n-button>
      <n-button size="small" :disabled="c.executing.value" @click="runExplain">{{ t('console.explain') }}</n-button>
      <n-button v-if="c.executing.value" size="small" type="error" @click="cancelRun">{{ t('console.cancel') }}</n-button>
      <n-button size="small" :disabled="c.executing.value" @click="formatEditor">{{ t('console.format') }}</n-button>
      <n-select class="console__history" size="small" clearable :value="null" :options="historyOptions" :placeholder="t('console.history')" @update:value="onHistoryPick" />
    </div>
    <p class="console__hint">{{ t('sqlWorkflows.hint') }}</p>
    <n-alert v-if="!c.draftSaved.value" type="warning">{{ t('sqlWorkflows.storageError') }}</n-alert>
    <SqlEditor ref="editor" v-model="c.sql.value" :dialect="flavor" :schema="completionSchema" :default-table="completionTable ?? undefined" :label="'SQL'" @execute="c.execute($event)" @history="cycleHistory" />
    <div class="console__toolbar">
      <label>{{ t('sqlWorkflows.completion') }}</label>
      <n-select class="console__table-picker" size="small" filterable clearable :value="completionTable" :options="tableOptions" :loading="completionLoading" @update:value="loadColumns" />
    </div>
    <details class="console__bookmarks">
      <summary>{{ t('sqlWorkflows.bookmarks') }}</summary>
      <p class="console__hint">{{ t('sqlWorkflows.limit') }} {{ t('sqlWorkflows.replaced') }}</p>
      <div class="console__toolbar">
        <n-select class="console__table-picker" size="small" filterable clearable :value="bookmarkName || null" :options="bookmarkOptions" :placeholder="t('sqlWorkflows.bookmarks')" @update:value="pickBookmark" />
        <n-input v-model:value="bookmarkName" class="console__bookmark-name" :input-props="{'aria-label':t('sqlWorkflows.name')}" :placeholder="t('sqlWorkflows.name')" :maxlength="100" size="small" />
        <n-button size="small" @click="storeBookmark">{{ t('sqlWorkflows.save') }}</n-button>
        <n-popconfirm v-if="bookmarks.some(b => b.name === bookmarkName)" @positive-click="deleteBookmark">
          <template #trigger><n-button size="small" type="error" secondary>{{ t('sqlWorkflows.remove') }}</n-button></template>
          {{ t('sqlWorkflows.remove') }}: {{ bookmarkName }}?
        </n-popconfirm>
      </div>
      <p role="status">{{ bookmarkNotice }}</p>
    </details>
    <n-alert v-if="c.error.value" type="error" class="console__error">{{ c.error.value }}</n-alert>
    <template v-if="c.results.value !== null">
      <p v-if="c.results.value.length === 0" class="console__empty">{{ t('console.results') }}: 0</p>
      <ResultGrid v-for="(res, i) in c.results.value" :key="i" :result="res" :index="i + 1" />
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { NAlert, NButton, NInput, NPopconfirm, NSelect } from 'naive-ui'
import SqlEditor from '../components/SqlEditor.vue'
import ResultGrid from '../components/ResultGrid.vue'
import { api } from '../api/client'
import type { TableDef } from '../api/types'
import { formatSQL } from '../lib/formatSQL'
import { useConsole } from '../lib/useConsole'
import { loadBookmarks, loadDraft, removeBookmark, saveBookmark, sqlScopeKey, starterQuery } from '../lib/sqlWorkspace'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
const {t} = useI18n(), route = useRoute(), session = useSessionStore(), catalog = useCatalogStore()
const db = computed(()=>String(route.params.db ?? ''))
const flavor = computed(()=>session.sessionInfo?.driver.flavor ?? 'mysql')
const driver = session.sessionInfo?.driver
const serverKey = sqlScopeKey({flavor:flavor.value,host:driver?.host ?? '',port:driver?.port,user:driver?.user ?? '',database:''})
const c = useConsole({db:()=>db.value,serverKey,schema:()=>catalog.schemaAware ? catalog.schema : null})
const editor = ref<InstanceType<typeof SqlEditor>|null>(null)
const completionSchema = ref<Record<string,string[]>>({})
const completionTable = ref<string|null>(null), completionLoading = ref(false)
const tableOptions = computed(()=>catalog.tables.map(table=>({label:table.name,value:table.name})))
let completionRequest = 0
async function loadColumns(table:string|null) {
  completionTable.value = table
  const request = ++completionRequest
  if (!table) { completionLoading.value=false;return }
  completionLoading.value=true
  try {
    const def = await api<TableDef>('GET',catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables/${encodeURIComponent(table)}`))
    if (request === completionRequest) completionSchema.value={...completionSchema.value,[table]:def.columns.map(col=>col.name)}
  } catch { /* SQL remains editable if metadata is not visible to this account. */ }
  finally { if(request === completionRequest) completionLoading.value=false }
}
watch(()=>[db.value,catalog.schema],async()=>{
  const request=++completionRequest
  completionSchema.value={};completionTable.value=null
  const contextTable=typeof route.query.table === 'string' ? route.query.table : null
  if (contextTable && loadDraft(c.draftKey.value) === null && !c.sql.value) c.sql.value=starterQuery(contextTable,flavor.value,catalog.schema)
  await catalog.fetchTables(db.value)
  if(request!==completionRequest)return
  completionSchema.value=Object.fromEntries(catalog.tables.map(table=>[table.name,[]]))
  await loadColumns(contextTable ?? catalog.tables[0]?.name ?? null)
},{immediate:true})
watch(()=>route.query.table, table=>{if(typeof table==='string')void loadColumns(table)})
onBeforeUnmount(()=>{completionRequest++})
const bookmarks=ref(loadBookmarks(c.draftKey.value)), bookmarkName=ref(''), bookmarkNotice=ref('')
watch(c.draftKey,()=>{bookmarks.value=loadBookmarks(c.draftKey.value);bookmarkName.value='';bookmarkNotice.value=''})
const bookmarkOptions=computed(()=>bookmarks.value.map(b=>({label:b.name,value:b.name})))
function pickBookmark(name:string|null) { bookmarkName.value=name??'';const b=bookmarks.value.find(b=>b.name===name);if(b)c.sql.value=b.sql }
function storeBookmark() {
  const ok=saveBookmark(c.draftKey.value,bookmarkName.value,c.sql.value)
  bookmarkNotice.value=t(ok?'sqlWorkflows.saved':'sqlWorkflows.saveFailed')
  bookmarks.value=loadBookmarks(c.draftKey.value)
}
function deleteBookmark() { if(!removeBookmark(c.draftKey.value,bookmarkName.value))bookmarkNotice.value=t('sqlWorkflows.saveFailed');bookmarks.value=loadBookmarks(c.draftKey.value) }
function runAll(){void c.execute()}
function runSelection(){void c.execute(editor.value?.selection())}
function runExplain(){void c.explain(editor.value?.selection())}
function cancelRun(){c.cancel().catch(()=>{})}
function formatEditor(){try{c.sql.value=formatSQL(c.sql.value,flavor.value)}catch(err){c.error.value=err instanceof Error?err.message:String(err)}}
function cycleHistory(direction:'older'|'newer'){const text=direction==='older'?c.historyPrev():c.historyNext();if(text!==null)c.sql.value=text}
const historyOptions=computed(()=>c.history.value.map((entry,i)=>({label:entry,value:String(i)})))
function onHistoryPick(value:string|null){if(value===null)return;const entry=c.history.value[Number(value)];if(entry!==undefined){c.resetHistoryCycle();c.sql.value=entry}}
</script>
<style scoped>
.console {display:flex;flex-direction:column;gap:12px;min-width:0}
h2 {margin:0;font-size:18px}
.console__toolbar {display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.console__history {min-width:200px;max-width:360px;margin-left:auto}
.console__table-picker {width:240px;max-width:100%}
.console__bookmark-name {width:220px;max-width:100%}
.console__hint {margin:0;font-size:12px;opacity:.7}
.console__bookmarks {border:1px solid #8884;border-radius:5px;padding:10px}
.console__bookmarks summary {cursor:pointer;margin-bottom:8px}
.console__empty {opacity:.7}
</style>

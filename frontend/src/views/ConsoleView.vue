<template>
  <section class="console tool-workspace db-page">
    <ToolHeading icon="terminal" :title="t('sqlWorkflows.title')" />
    <ui-alert v-if="!c.draftSaved.value" type="warning">{{ t('sqlWorkflows.storageError') }}</ui-alert>
    <section class="tool-card query-card">
      <header class="tool-card__head">
        <h2>{{ t('sqlWorkflows.query') }}</h2>
        <div class="tool-actions console__editor-tools">
          <ui-button size="small" quaternary :disabled="c.executing.value" @click="formatEditor">
            {{ t('console.format') }}
          </ui-button>
          <ui-select
            class="console__history"
            size="small"
            clearable
            :value="null"
            :options="historyOptions"
            :placeholder="t('console.history')"
            :aria-label="t('console.history')"
            @update:value="onHistoryPick"
          />
        </div>
      </header>
      <SqlEditor
        ref="editor"
        v-model="c.sql.value"
        :dialect="flavor"
        :schema="completionSchema"
        :default-table="completionTable ?? undefined"
        label="SQL"
        @execute="c.execute($event)"
        @history="cycleHistory"
      />
      <footer class="tool-card__foot">
        <div class="tool-actions">
          <ui-button size="small" type="primary" :loading="c.executing.value" @click="runAll">
            <template #icon><UiIcon name="play" /></template>
            {{ t('console.executeAll') }}
          </ui-button>
          <ui-button size="small" :disabled="c.executing.value" @click="runSelection">
            {{ t('console.executeSelection') }}
          </ui-button>
          <ui-button size="small" :disabled="c.executing.value" @click="runExplain">
            {{ t('console.explain') }}
          </ui-button>
          <ui-button v-if="c.executing.value" size="small" type="error" @click="cancelRun">
            {{ t('console.cancel') }}
          </ui-button>
        </div>
        <span class="tool-note"><span class="console__shortcut">{{ t('sqlWorkflows.shortcut') }}{{ ' ' }}</span>{{ t('sqlWorkflows.draftHint') }}</span>
      </footer>
    </section>
    <div class="console__aux">
      <details class="tool-card tool-details console__bookmarks">
        <summary>{{ t('sqlWorkflows.bookmarks') }}</summary>
        <div class="tool-stack tool-card__body">
          <div class="tool-actions">
            <ui-select
              class="console__table-picker"
              size="small"
              filterable
              clearable
              :value="bookmarkName || null"
              :options="bookmarkOptions"
              :placeholder="t('sqlWorkflows.bookmarks')"
              :aria-label="t('sqlWorkflows.bookmarks')"
              @update:value="pickBookmark"
            />
            <ui-input
              v-model:value="bookmarkName"
              class="console__bookmark-name"
              :input-props="{ 'aria-label': t('sqlWorkflows.name') }"
              :placeholder="t('sqlWorkflows.name')"
              :maxlength="100"
              size="small"
            />
            <ui-button size="small" @click="storeBookmark">{{ t('sqlWorkflows.save') }}</ui-button>
            <ui-popconfirm
              v-if="bookmarks.some((b) => b.name === bookmarkName)"
              @positive-click="deleteBookmark"
            >
              <template #trigger>
                <ui-button size="small" type="error" secondary>{{ t('sqlWorkflows.remove') }}</ui-button>
              </template>
              {{ t('sqlWorkflows.remove') }} · {{ bookmarkName }}?
            </ui-popconfirm>
          </div>
          <p class="tool-note">{{ t('sqlWorkflows.limit') }} {{ t('sqlWorkflows.replaced') }}</p>
          <p v-if="bookmarkNotice" role="status">{{ bookmarkNotice }}</p>
        </div>
      </details>
      <details class="tool-card tool-details">
        <summary>{{ t('sqlWorkflows.completion') }}</summary>
        <div class="tool-card__body">
          <ui-select
            class="console__table-picker"
            size="small"
            filterable
            clearable
            :value="completionTable"
            :options="tableOptions"
            :loading="completionLoading"
            :aria-label="t('sqlWorkflows.completion')"
            @update:value="loadColumns"
          />
        </div>
      </details>
    </div>
    <ui-alert v-if="c.error.value" type="error" class="console__error">{{ c.error.value }}</ui-alert>
    <section v-if="!c.error.value" class="console__results tool-stack">
      <h2>{{ t('console.results') }}</h2>
      <ResultGrid v-for="(res, i) in c.results.value ?? []" :key="i" :result="res" :index="i + 1" />
      <p v-if="c.executing.value" class="tool-card tool-empty" role="status">
        {{ t('sqlWorkflows.executing') }}
      </p>
      <p v-else-if="!c.results.value?.length" class="tool-card tool-empty">
        {{ t(c.results.value === null ? 'sqlWorkflows.waiting' : 'sqlWorkflows.noResults') }}
      </p>
    </section>
  </section>
</template>

<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { UiAlert, UiButton, UiInput, UiPopconfirm, UiSelect } from '../ui'
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
const tables = computed(() => catalog.tablesFor(db.value))
const tableOptions = computed(()=>tables.value.map(table=>({label:table.name,value:table.name})))
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
  completionSchema.value=Object.fromEntries(tables.value.map(table=>[table.name,[]]))
  await loadColumns(contextTable ?? tables.value[0]?.name ?? null)
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
.query-card .console__editor-tools {
  display: grid;
  grid-template-columns: max-content minmax(100px, 190px);
  max-width: 100%;
}
.console__history { min-width: 0; }
.console__table-picker,
.console__bookmark-name {
  width: min(250px, 100%);
}
.console__aux {
  display: grid;
  grid-template-columns: 1fr 1fr;
  align-items: start;
  gap: 12px;
}
.query-card :deep(.sql-editor) {
  border: 0;
  border-radius: 0;
}
.query-card .tool-card__foot {
  justify-content: start;
}
.console__results {
  gap: 10px;
}
@media (max-width: 600px) {
  .query-card .console__editor-tools { grid-template-columns: max-content minmax(95px, 1fr); }
  .query-card .tool-card__head { gap: 6px 12px; }
  .query-card .tool-card__foot .tool-actions { gap: 5px; }
  .query-card .tool-card__foot .ui-button { padding-inline: 6px; }
}
@media (max-width: 370px) {
  .query-card .tool-card__foot .tool-actions { display: grid; grid-template-columns: 1fr auto; width: 100%; }
  .query-card .tool-card__foot .ui-button--primary { grid-column: 1 / -1; }
}
@media (pointer: coarse) {
  .console__shortcut { display: none; }
}
@media (max-width: 780px) {
  .console__aux {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

<template>
  <section class="objects-view db-page tool-workspace">
    <ToolHeading
      :icon="kind === 'views' ? 'eye' : kind === 'partitions' ? 'columns' : 'code'"
      :title="t(`objects.${kind}`)"
    >
      <ui-button size="small" :disabled="busy || !supported" @click="refresh">
        <template #icon><UiIcon name="refresh" /></template>
        {{ t('objects.refresh') }}
      </ui-button>
      <ui-button
        v-if="supported && kind !== 'partitions'"
        type="primary"
        size="small"
        :disabled="busy"
        @click="newObject"
      >
        <template #icon><UiIcon name="plus" /></template>
        {{ t('objects.create') }}
      </ui-button>
    </ToolHeading>
    <p v-if="!supported" class="tool-card tool-empty" role="status">{{ t('objects.unsupported') }}</p>
    <template v-else>
      <p v-if="error && !pending" role="alert" class="error">{{ error }}</p>
      <p v-if="kind === 'partitions' && !catalog.loading && catalog.error" role="alert" class="error">{{ catalog.error }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <section v-if="kind === 'partitions'" class="tool-card">
        <header class="tool-card__head">
          <label class="table-picker">
            {{ t('objects.table') }}
            <select :value="table" @change="changeTable">
              <option value="">{{ t('objects.selectTable') }}</option>
              <option
                v-for="item in partitionTables"
                :key="item.name"
                :value="item.name"
              >
                {{ item.name }}
              </option>
            </select>
          </label>
        </header>
        <div v-if="table" class="tool-scroll admin-records">
          <table class="catalog-table tool-grid">
            <thead>
              <tr>
                <th>{{ t('objects.name') }}</th>
                <th>{{ t('objects.method') }}</th>
                <th>{{ t('objects.expression') }}</th>
                <th>{{ t('objects.boundary') }}</th>
                <th>{{ t('objects.rows') }}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="part in partitions" :key="part.name">
                <td :data-field="t('objects.name')">{{ part.name }}</td>
                <td :data-field="t('objects.method')">{{ part.method }}</td>
                <td :data-field="t('objects.expression')">{{ part.expression }}</td>
                <td :data-field="t('objects.boundary')">{{ part.description }}</td>
                <td :data-field="t('objects.rows')">{{ part.rows }}</td>
                <td class="record-wide">
                  <div class="tool-actions">
                    <ui-button size="tiny" :disabled="busy" @click="scheduleOperation('truncate', part.name)">
                      {{ t('objects.truncate') }}
                    </ui-button>
                    <ui-button
                      size="tiny"
                      secondary
                      type="error"
                      :data-partition-drop="part.name"
                      :disabled="busy"
                      @click="scheduleOperation('partition-drop', part.name)"
                    >
                      {{ t('objects.remove') }}
                    </ui-button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="tool-card__body tool-stack">
          <label class="tool-field">
            {{ t('objects.partitionSource') }}
            <textarea v-model="source" data-source rows="4" :disabled="busy || !table"></textarea>
          </label>
          <p class="tool-note">{{ t('objects.partitionHint') }}</p>
          <div class="tool-actions">
            <ui-button data-save type="primary" :disabled="busy || !table || !source.trim()" @click="save">
              {{ t('objects.addPartition') }}
            </ui-button>
          </div>
        </div>
      </section>
      <div v-else class="object-workspace">
        <aside class="tool-card object-catalog">
          <div class="tool-card__head">
            <input
              v-model="filter"
              type="search"
              :aria-label="t('objects.search')"
              :placeholder="t('objects.search')"
            />
          </div>
          <p v-if="!filteredObjects.length" class="tool-empty">{{ t('objects.empty') }}</p>
          <ul v-else>
            <li v-for="item in filteredObjects" :key="`${item.name}|${item.identity}|${item.table}`">
              <button
                type="button"
                :data-object="item.name"
                :aria-pressed="selected === item"
                :disabled="busy"
                @click="selectObject(item)"
              >
                <UiIcon :name="kind === 'views' ? 'eye' : 'code'" :size="16" />
                <span>
                  <strong>{{ item.name }}</strong>
                  <small>{{ [item.kind, item.identity, item.table].filter(Boolean).join(' · ') }}</small>
                </span>
              </button>
            </li>
          </ul>
        </aside>
        <article v-if="editing" class="tool-card object-editor">
          <div class="tool-card__body tool-stack">
            <div class="tool-fields">
              <label class="tool-field">
                {{ t('objects.name') }}
                <input v-model="name" :disabled="busy || !!selected" data-name />
              </label>
              <label v-if="kind === 'routines'" class="tool-field">
                {{ t('objects.kind') }}
                <select v-model="routineKind" :disabled="busy || !!selected">
                  <option value="FUNCTION">FUNCTION</option>
                  <option v-if="session.sessionInfo?.caps.procedures" value="PROCEDURE">PROCEDURE</option>
                </select>
              </label>
              <label v-if="kind === 'triggers'" class="tool-field">
                {{ t('objects.table') }}
                <input v-model="objectTable" :disabled="busy" />
              </label>
            </div>
            <div class="tool-field">
              {{ t('objects.source') }}
              <SqlEditor
                v-model="source"
                :dialect="session.sessionInfo?.driver.flavor"
                :label="t('objects.source')"
                :disabled="busy"
              />
            </div>
            <p class="tool-note">{{ t('objects.sourceHint') }}</p>
          </div>
          <div class="tool-card__foot">
            <ui-button data-save type="primary" :disabled="busy || !name || !source.trim()" @click="save">
              {{ t('objects.save') }}
            </ui-button>
            <ui-button
              v-if="selected"
              data-delete
              secondary
              type="error"
              :disabled="busy"
              @click="scheduleOperation('object-drop', name)"
            >
              {{ t('objects.remove') }}
            </ui-button>
          </div>
          <section
            v-if="selected?.kind === 'PROCEDURE' && session.sessionInfo?.caps.procedures"
            class="procedure-call tool-stack tool-card__body"
          >
            <label class="tool-field">
              {{ t('objects.parameters') }}
              <textarea v-model="argumentsJSON" data-arguments rows="3" :disabled="busy"></textarea>
            </label>
            <p class="tool-note">{{ t('objects.parameterHint') }}</p>
            <div class="tool-actions">
              <ui-button data-call :disabled="busy" @click="call">{{ t('objects.call') }}</ui-button>
            </div>
            <ResultGrid v-for="(result, index) in results" :key="index" :result="result" :index="index + 1" />
          </section>
        </article>
        <div v-else class="tool-card tool-empty object-empty">
          <UiIcon name="code" :size="25" />
          <p>{{ t('objects.select') }}</p>
        </div>
      </div>
      <UiModal :show="!!pending" preset="dialog"
        :title="t(pending?.operation === 'truncate' ? 'objects.truncate' : 'objects.remove')"
        :closable="!busy" :close-on-esc="!busy" :mask-closable="!busy" @update:show="pending = null">
        <p v-if="pending">
          {{ t(pending.operation === 'object-drop' ? 'objects.deleteConfirm' : 'objects.partitionConfirm') }}
          <strong>{{ pending.label }}</strong>
        </p>
        <p v-if="error" role="alert" class="error">{{ error }}</p>
        <template #footer>
          <ui-button data-cancel :disabled="busy" @click="pending = null">{{ t('objects.cancel') }}</ui-button>
          <ui-button :disabled="busy" data-confirm type="error" @click="confirmOperation">
            {{ t(pending?.operation === 'truncate' ? 'objects.truncate' : 'objects.remove') }}
          </ui-button>
        </template>
      </UiModal>
    </template>
  </section>
</template>

<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, ref, watch } from 'vue'
import { UiButton, UiModal } from '../ui'
import SqlEditor from '../components/SqlEditor.vue'
import { useDirtyGuard } from '../lib/useDirtyGuard'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { QueryResult } from '../api/types'
import ResultGrid from '../components/ResultGrid.vue'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'

interface DbObject { name: string; kind: string; source: string; table?: string; identity?: string }
interface Partition { name: string; method: string; expression: string; description: string; rows: number }
const { t } = useI18n()
const route = useRoute(), session = useSessionStore(), catalog = useCatalogStore()
const kind = computed(() => route.params.t ? 'partitions' : String(route.params.kind))
const supported = computed(() => ['views', 'triggers', 'routines', 'events', 'partitions'].includes(kind.value) && Boolean(session.sessionInfo?.caps[kind.value as keyof typeof session.sessionInfo.caps]))
const db = computed(() => String(route.params.db ?? ''))
const table = ref('')
const tablesPath = computed(() => catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables`))
const partitionTables = computed(() => catalog.tablesPath === tablesPath.value ? catalog.tables.filter(item => !item.isView) : [])
const base = computed(() => `/api/databases/${encodeURIComponent(db.value)}/${kind.value === 'partitions' ? `tables/${encodeURIComponent(table.value)}/partitions` : kind.value}`)
const objects = ref<DbObject[]>([]), partitions = ref<Partition[]>([]), selected = ref<DbObject | null>(null)
const editing = ref(false), busy = ref(false), error = ref(''), notice = ref(''), filter = ref(''), name = ref(''), source = ref(''), objectTable = ref(''), routineKind = ref('FUNCTION'), argumentsJSON = ref('[]'), results = ref<QueryResult[]>([])
type PendingOperation = { operation: 'object-drop' | 'partition-drop' | 'truncate'; path: string; label: string }
const pending = ref<PendingOperation | null>(null)
const filteredObjects = computed(() => objects.value.filter(o => o.name.toLowerCase().includes(filter.value.toLowerCase())))
let request = 0
function endpoint(suffix = '') { return catalog.path(base.value + suffix) }
const baseline = ref('')
function snapshot() { return kind.value === 'partitions' ? source.value : JSON.stringify([name.value,source.value,objectTable.value,routineKind.value]) }
const discard = useDirtyGuard(()=>(kind.value === 'partitions' || editing.value) && snapshot() !== baseline.value,()=>t('sqlWorkflows.discard'))
function refresh() {
 if (!discard()) return
 if (kind.value === 'partitions' && (catalog.error || catalog.tablesPath !== tablesPath.value)) void catalog.fetchTables(db.value)
 void load()
}
function changeTable(event: Event) {
 const select = event.target as HTMLSelectElement
 if (!discard()) { select.value = table.value; return }
 table.value = select.value
}
function newObject() { if(!discard())return; selected.value = null; editing.value = true; name.value = ''; source.value = ''; objectTable.value = ''; routineKind.value = 'FUNCTION'; results.value = []; notice.value = ''; error.value = ''; baseline.value=snapshot() }
function selectObject(item: DbObject) { if(!discard())return; selected.value = item; editing.value = true; name.value = item.name; source.value = item.source; objectTable.value = item.table ?? ''; routineKind.value = item.kind; results.value = []; notice.value = ''; error.value = ''; baseline.value=snapshot() }
async function load() {
 const current = ++request
 objects.value = []; partitions.value = []; selected.value = null; editing.value = false; pending.value = null; results.value = []; source.value = ''; error.value = ''; baseline.value = snapshot()
 if (!supported.value || (kind.value === 'partitions' && !table.value)) { busy.value = false; return }
 busy.value = true
 try { const body = await api<{ objects?: DbObject[]; partitions?: Partition[] }>('GET', endpoint()); if (current !== request) return; objects.value = body.objects ?? []; partitions.value = body.partitions ?? [] }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
async function mutate(action: () => Promise<unknown>, message: string) {
 if (busy.value) return
 busy.value = true; error.value = ''; notice.value = ''; const current = request
 try { await action(); if (current !== request) return; await load(); notice.value = t(message) }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
function save() {
 const path = endpoint(kind.value === 'partitions' ? '' : `/${encodeURIComponent(name.value)}`)
 const payload = { source: source.value, kind: routineKind.value, table: objectTable.value, identity: selected.value?.identity ?? '' }
 void mutate(() => api(kind.value === 'partitions' ? 'POST' : 'PUT', path, payload), 'objects.saved')
}
function scheduleOperation(operation: PendingOperation['operation'], targetName: string) {
 let suffix = `/${encodeURIComponent(targetName)}`
 let label = targetName
 if (operation === 'truncate') {
  suffix += '/truncate'
  label += ` (${table.value})`
 }
 else if (operation === 'object-drop') {
  const object = selected.value
  if (!object) return
  suffix += `?${new URLSearchParams({ kind: object.kind, table: object.table ?? '', identity: object.identity ?? '' })}`
  label += [object.table, object.identity].filter(Boolean).map(value => ` (${value})`).join('')
 } else {
  label += ` (${table.value})`
 }
 pending.value = { operation, path: endpoint(suffix), label }
}
function confirmOperation() {
 const operation = pending.value; if (!operation || !discard()) return
 void mutate(() => api(operation.operation === 'truncate' ? 'POST' : 'DELETE', operation.path), 'objects.deleted')
}
async function call() {
 let args: unknown
 try { args = JSON.parse(argumentsJSON.value); if (!Array.isArray(args) || args.some(v => v !== null && !['string', 'number', 'boolean'].includes(typeof v))) throw new Error() }
 catch { error.value = t('objects.invalidArgs'); return }
 const path = endpoint(`/${encodeURIComponent(name.value)}/call`), identity = selected.value?.identity ?? '', current = request
 busy.value = true; error.value = ''; results.value = []
 try { const body = await api<{ results: QueryResult[] }>('POST', path, { args, identity }); if (current === request) results.value = body.results }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
watch([tablesPath, kind, supported, () => route.params.t], () => {
 if (kind.value !== 'partitions' || !supported.value) return
 table.value = String(route.params.t ?? partitionTables.value[0]?.name ?? '')
 if (catalog.tablesPath !== tablesPath.value) void catalog.fetchTables(db.value)
}, { immediate: true })
watch(partitionTables, items => {
 if (kind.value === 'partitions' && !route.params.t && !table.value && items.length) table.value = items[0]!.name
})
watch([tablesPath, kind, supported, table], () => { void load() }, { immediate: true })
</script>

<style scoped src="./adminRecords.css"></style>
<style scoped>
.object-workspace {
  display: grid;
  grid-template-columns: minmax(175px, 27%) minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.object-catalog .tool-card__head {
  padding: 10px;
}
.object-catalog input {
  width: 100%;
}
.object-catalog ul {
  margin: 0;
  padding: 6px;
  list-style: none;
  max-height: 55vh;
  overflow: auto;
}
.object-catalog li button {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  text-align: start;
  padding: 9px;
  color: var(--db-ink);
  background: transparent;
  border: 0;
  border-radius: 6px;
  font: inherit;
  cursor: pointer;
}
.object-catalog li button:hover {
  background: var(--db-sidebar);
}
.object-catalog li button[aria-pressed="true"] {
  background: var(--db-tint);
  color: var(--db-accent);
}
.object-catalog li strong,
.object-catalog li small {
  display: block;
  overflow-wrap: anywhere;
}
.object-catalog li strong {
  font: 600 13px var(--db-mono);
}
.object-catalog li small {
  font-size: 11px;
  color: var(--db-muted);
  margin-top: 3px;
}
.object-empty {
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 10px;
  min-height: 160px;
}
.object-editor {
  min-width: 0;
}
.table-picker {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
  min-width: 0;
}
.procedure-call {
  border-top: 1px solid var(--db-line);
}
textarea {
  font-family: var(--db-mono);
  resize: vertical;
}
.error {
  color: var(--db-danger, #bc5555);
  overflow-wrap: anywhere;
}
@media (max-width: 820px) {
  .object-workspace {
    grid-template-columns: minmax(0, 1fr);
  }
  .object-catalog ul {
    max-height: 210px;
  }
}
@container records (max-width: 660px) {
  .catalog-table .tool-actions { margin-top: 2px; }
}
</style>

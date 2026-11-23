<template>
  <section class="tool-card tool-card__body tool-stack">
    <h3>{{ t('roles.rights') }}</h3>
    <p v-if="error" role="alert">{{ error }}</p>
    <fieldset class="selectors" :disabled="busy">
      <label>{{ t('admin.database') }}
        <select v-model="database" @change="chooseDatabase">
          <option value="" disabled>{{ t('roles.choose') }}</option>
          <option v-for="db in databases" :key="db" :value="db">{{ db }}</option>
        </select>
      </label>
      <label v-if="database">{{ t('admin.scope') }}
        <select v-model="kind" @change="chooseKind">
          <option value="database">{{ t('admin.database') }}</option>
          <option value="schema">{{ t('roles.schema') }}</option>
          <option value="table">{{ t('admin.table') }}</option>
        </select>
      </label>
      <label v-if="database && kind !== 'database'">{{ t('roles.schema') }}
        <select v-model="schema" @change="chooseSchema">
          <option value="" disabled>{{ t('roles.choose') }}</option>
          <option v-for="s in schemas" :key="s" :value="s">{{ s }}</option>
        </select>
      </label>
      <label v-if="schema && kind === 'table'">{{ t('admin.table') }}
        <select v-model="table" @change="refresh">
          <option value="" disabled>{{ t('roles.choose') }}</option>
          <option v-for="item in tables" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
    </fieldset>
    <table v-if="ready" class="tool-grid">
      <thead><tr><th>{{ t('roles.privilege') }}</th><th>{{ t('roles.source') }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="p in privileges" :key="p">
          <td>{{ p }}</td>
          <td>{{ sources(p).join(', ') || '—' }}</td>
          <td><UiButton v-if="editable.includes(p)" size="small" :disabled="busy" @click="apply(p)">
            {{ t(direct(p) ? 'admin.toRevoke' : 'admin.toGrant') }}
          </UiButton></td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import type { DatabaseInfo, TableInfo } from '../api/types'
import { useSessionStore } from '../stores/session'
import { UiButton } from '../ui'

interface Grant { privilege: string; source: string; public: boolean }
const props = defineProps<{ role: string; revision: number }>()
const session = useSessionStore()
const { t } = useI18n()
const database = ref('')
const kind = ref<'database' | 'schema' | 'table'>('database')
const schema = ref('')
const table = ref('')
const databases = ref<string[]>([])
const schemas = ref<string[]>([])
const tables = ref<string[]>([])
const grants = ref<Grant[]>([])
const error = ref('')
const busy = ref(false)
const ready = ref(false)
const base = computed(() => '/api/databases/' + encodeURIComponent(database.value))
const editable = ref<string[]>([])
const privileges = computed(() => [...new Set([...editable.value, ...grants.value.map(g => g.privilege)])])
function direct(p: string) { return grants.value.some(g => g.privilege === p && g.source === props.role && !g.public) }
function sources(p: string) {
  return grants.value.filter(g => g.privilege === p).map(g => g.public ? 'PUBLIC' : g.source === props.role ? t('roles.direct') : t('roles.inherited', { name: g.source }))
}
function object() { return { kind: kind.value, schema: schema.value, name: table.value } }
async function read() {
  ready.value = false
  if (!database.value || (kind.value !== 'database' && !schema.value) || (kind.value === 'table' && !table.value)) return
  const query = new URLSearchParams({ role: props.role, ...object() })
  const result = await api<{ grants: Grant[]; editable: string[] }>('GET', base.value + '/role-privileges?' + query)
  grants.value = result.grants
  editable.value = result.editable
  ready.value = true
}
let pendingRefresh = false
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await action() }
  catch (err) { error.value = err instanceof Error ? err.message : String(err) }
  finally {
    busy.value = false
    if (pendingRefresh) { pendingRefresh = false; await refresh() }
  }
}
function refresh(): Promise<void> {
  if (busy.value) { pendingRefresh = true; return Promise.resolve() }
  return run(read)
}
function chooseDatabase() {
  schema.value = table.value = ''
  schemas.value = tables.value = []
  kind.value = 'database'
  return refresh()
}
function chooseKind() {
  schema.value = table.value = ''
  schemas.value = tables.value = []
  ready.value = false
  return run(async () => {
    if (kind.value !== 'database') {
      schemas.value = (await api<{ schemas: string[] }>('GET', base.value + '/schemas')).schemas
    }
    await read()
  })
}
function chooseSchema() {
  table.value = ''
  tables.value = []
  ready.value = false
  return run(async () => {
    if (kind.value === 'table') {
      const result = await api<{ tables: TableInfo[] }>('GET', base.value + '/tables?schema=' + encodeURIComponent(schema.value))
      tables.value = result.tables.map(item => item.name)
    }
    await read()
  })
}
function apply(privilege: string) {
  return run(async () => {
    await api('POST', base.value + '/role-privileges', { role: props.role, object: object(), privilege, grant: !direct(privilege) })
    await read()
  })
}
function initialize() {
  return run(async () => {
    const result = await api<{ databases: DatabaseInfo[] }>('GET', '/api/databases')
    databases.value = result.databases.map(db => db.name)
    const current = session.sessionInfo?.database
    if (current && databases.value.includes(current)) database.value = current
    await read()
  })
}
onMounted(initialize)
watch(() => props.revision, () => databases.value.length ? refresh() : initialize())
</script>
<style scoped>
.selectors { display: flex; flex-wrap: wrap; align-items: end; gap: 12px; padding: 0; margin: 0; border: 0; min-width: 0; }
.selectors label { display: grid; gap: 4px; max-width: 100%; }
select { max-width: 100%; }
.tool-grid td { overflow-wrap: anywhere; }
[role="alert"] { color: var(--db-danger); }
</style>

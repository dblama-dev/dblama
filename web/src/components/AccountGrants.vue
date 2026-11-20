<template>
  <section class="grant-workspace" :aria-label="t('admin.privileges')">
    <p v-if="error" role="alert">{{ error }}</p>
    <template v-if="grants === null">
      <span v-if="busy" role="status">{{ t('admin.loading') }}</span>
      <n-button v-else data-test="retry-grants" @click="refresh">{{ t('admin.refresh') }}</n-button>
    </template>
    <template v-else>
      <div class="section-header">
        <h4>{{ t('admin.assignedScopes') }}</h4>
        <n-button data-test="choose-scope" :disabled="busy" @click="chooseScope">{{ t('admin.chooseScope') }}</n-button>
      </div>
      <ul class="scope-list">
        <li v-for="group in groups" :key="scopeKey(group.scope)">
          <button v-if="group.scope.level" type="button" data-test="open-scope" :aria-pressed="sameScope(group.scope, scope) && !choosing"
            :disabled="busy" @click="openScope(group.scope)">
            <strong>{{ scopeTitle(group.scope) }}</strong>
            <span class="scope-description">{{ group.description }}</span>
          </button>
          <div v-else class="unknown"><strong>{{ t('admin.otherGrants') }}</strong><span>{{ t('admin.readOnly') }}</span></div>
        </li>
      </ul>
      <form v-if="choosing" class="scope-picker" @submit.prevent="openDraftScope">
        <label>{{ t('admin.scope') }}
          <select v-model="draftLevel" data-test="scope-level">
            <option value="database">{{ t('admin.database') }}</option>
            <option v-if="databasePatterns" value="databasePattern">{{ t('admin.databasePattern') }}</option>
            <option value="table">{{ t('admin.table') }}</option>
            <option value="tablePattern">{{ t('admin.tablePattern') }}</option>
            <option value="global">{{ t('admin.global') }}</option>
          </select>
        </label>
        <label v-if="draftLevel === 'databasePattern'">{{ t('admin.databasePattern') }}
          <input v-model="draftPattern" data-test="scope-database-pattern" aria-describedby="grant-pattern-help" :aria-invalid="!!draftPattern && !validDraft"
            required autocomplete="off" spellcheck="false">
        </label>
        <label v-else-if="draftLevel !== 'global'">{{ t('admin.database') }}
          <input v-model="draftDatabase" data-test="scope-database" list="grant-databases" required autocomplete="off" :placeholder="t('admin.exactName')">
          <datalist id="grant-databases"><option v-for="db in databases" :key="db" :value="db"/></datalist>
        </label>
        <label v-if="draftLevel === 'table'">{{ t('admin.table') }}
          <input v-model="draftTable" data-test="scope-table" list="grant-tables" required autocomplete="off" :placeholder="t('admin.exactName')">
          <datalist id="grant-tables"><option v-for="table in tables" :key="table" :value="table"/></datalist>
        </label>
        <n-button v-if="databasePatterns && username && ['database', 'databasePattern'].includes(draftLevel)"
          data-test="username-database-pattern" class="username-pattern" @click="useUsernamePattern">{{ t('admin.usernameDatabasePattern', { pattern: usernamePattern }) }}</n-button>
        <p v-if="draftLevel === 'databasePattern'" id="grant-pattern-help" class="picker-help">{{ t('admin.databasePatternHelp') }}</p>
        <n-button attr-type="submit" data-test="open-new-scope" :disabled="!validDraft">
          {{ t(draftLevel === 'tablePattern' ? 'admin.previewTables' : 'admin.openScope') }}
        </n-button>
        <n-button @click="choosing = false">{{ t('admin.cancel') }}</n-button>
      </form>
      <template v-else-if="scope.level !== 'tablePattern'">
        <div class="section-header">
          <div><h4>{{ t('admin.privileges') }}</h4><span class="scope-caption">{{ scopeTitle(scope) }}</span></div>
          <input v-if="editable" v-model="filter" type="search" :aria-label="t('admin.findPrivilege')" :placeholder="t('admin.findPrivilege')">
        </div>
        <p v-if="scope.level === 'databasePattern'" class="hint">{{ t('admin.databasePatternApplies') }}</p>
        <p v-if="!catalog.length" class="hint">{{ t('admin.missingCatalog') }}</p>
        <div v-else-if="scopeReadOnly" class="readonly">
          <p>{{ t('admin.unsupportedGrants') }}</p>
          <code v-for="(entry, i) in scoped" :key="i">{{ entry.privileges.join(', ') }} {{ entry.grantOption ? 'WITH GRANT OPTION' : '' }}</code>
        </div>
        <div v-else class="privilege-groups">
          <label v-if="scope.level === 'databasePattern' && available.length" class="check">
            <input type="checkbox" data-test="toggle-database-pattern-privileges" :checked="desired.length === available.length"
              :indeterminate="desired.length > 0 && desired.length < available.length" :disabled="busy"
              @change="desired = ($event.target as HTMLInputElement).checked ? available.map(p => p.name) : []">
            {{ t('admin.allDatabasePatternPrivileges') }}
          </label>
          <section v-for="group in privilegeGroups" :key="group.key" class="privilege-group" :data-grant-group="group.key"
            :aria-labelledby="'privilege-heading-' + group.key">
            <div class="group-heading">
              <h5 :id="'privilege-heading-' + group.key">{{ t('admin.grantGroups.' + group.key) }}</h5>
              <label v-if="group.key === 'data'" class="check">
                <input type="checkbox" data-test="toggle-shown-data" :checked="allShownData" :indeterminate="someShownData && !allShownData"
                  :disabled="busy" @change="toggleShownData(($event.target as HTMLInputElement).checked)">
                {{ t('admin.selectShownData', { count: group.names.length }) }}
              </label>
            </div>
            <div class="privilege-matrix">
              <label v-for="name in group.names" :key="name">
                <input v-model="desired" :data-test="'privilege-' + name" type="checkbox" :value="name" :disabled="busy">
                <code>{{ name }}</code>
              </label>
            </div>
          </section>
          <p v-if="!privilegeGroups.length">{{ t('admin.noMatchingPrivileges') }}</p>
        </div>
        <p v-if="editable && separateOptions">{{ t('admin.separateOptions') }} <code>{{ delegatedNames.join(', ') || '—' }}</code></p>
        <label v-else-if="editable" class="check"><input v-model="grantOption" data-test="grant-option" type="checkbox" :disabled="busy">{{ t('admin.grantOption') }}</label>
        <div v-if="dirty" data-test="pending-grants" role="status" class="pending">
          <strong>{{ t('admin.pendingGrants') }}</strong>
          <span v-if="added.length || addOption">{{ t('admin.toGrant') }} · {{ [...added, ...(addOption ? ['GRANT OPTION'] : [])].join(', ') }}</span>
          <span v-if="removed.length || removeOption">{{ t('admin.toRevoke') }} · {{ [...removed, ...(removeOption ? ['GRANT OPTION'] : [])].join(', ') }}</span>
        </div>
        <div class="actions">
          <n-button type="primary" data-test="save-grants" :disabled="busy || !editable || !dirty" @click="save">{{ t('admin.saveGrants') }}</n-button>
          <n-button v-if="dirty" :disabled="busy" @click="reset">{{ t('admin.cancel') }}</n-button>
          <span v-if="saved" role="status">{{ t('admin.saved') }}</span>
        </div>
      </template>
    </template>
    <template v-if="!choosing && scope.level === 'tablePattern' && scope.database">
      <h4>{{ scopeTitle(scope) }}</h4>
      <TablePatternGrants :key="scope.database" :database="scope.database" :username="username" :path="path" :catalog="catalog"
        :disabled="requestBusy || grants === null" @busy="tablePatternBusy = $event" @dirty="tablePatternDirty = $event" @changed="refresh"/>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, useThemeVars } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { usernamePrefixPattern, validGrantPattern } from '../lib/grantPattern'
import TablePatternGrants from './TablePatternGrants.vue'

interface Scope { level: string; database?: string; table?: string }
interface Grant { scope: Scope; privileges: string[]; grantOption: boolean; editable: boolean }
const props = defineProps<{
  path: string
  catalog: { name: string; levels: string[] }[]
  newAccount: boolean
  username: string
  databasePatterns: boolean
}>()
const emit = defineEmits<{ busy: [boolean]; dirty: [boolean] }>()
const { t } = useI18n()
const theme = useThemeVars()
const grants = ref<Grant[] | null>(null)
const scope = ref<Scope>({ level: 'global' })
const desired = ref<string[]>([])
const grantOption = ref(false)
const filter = ref('')
const requestBusy = ref(false)
const tablePatternBusy = ref(false)
const busy = computed(() => requestBusy.value || tablePatternBusy.value)
const error = ref('')
const saved = ref(false)
const choosing = ref(false)
const draftLevel = ref('database')
const draftDatabase = ref('')
const draftPattern = ref('')
const draftTable = ref('')
const databases = ref<string[]>([])
const tables = ref<string[]>([])
const tablePatternDirty = ref(false)
const usernamePattern = computed(() => usernamePrefixPattern(props.username))

function scopeKey(s: Scope) { return JSON.stringify([s.level, s.database, s.table]) }
function sameScope(a: Scope, b: Scope) { return scopeKey(a) === scopeKey(b) }
function scopeTitle(s: Scope) {
  if (s.level === 'global') return t('admin.global') + ' · *.*'
  if (s.level === 'database') return t('admin.database') + ' · ' + s.database
  if (s.level === 'databasePattern') return t('admin.databasePattern') + ' · ' + s.database
  if (s.level === 'tablePattern') return t('admin.tablePattern') + ' · ' + s.database
  return t('admin.table') + ' · ' + s.database + '.' + s.table
}
const groups = computed(() => {
  const grouped = new Map<string, { scope: Scope; entries: Grant[] }>()
  for (const grant of grants.value ?? []) {
    const key = scopeKey(grant.scope)
    const group = grouped.get(key) ?? { scope: grant.scope, entries: [] as Grant[] }
    group.entries.push(grant)
    grouped.set(key, group)
  }
  return Array.from(grouped.values()).map(({ scope, entries }) => {
    const names = new Set(entries.flatMap(g => g.privileges.filter(p => p !== 'USAGE')))
    if (entries.some(g => g.grantOption)) names.add('GRANT OPTION')
    const description = [...names].join(', ') || (entries.every(g => g.editable) ? t('admin.noPrivileges') : t('admin.readOnly'))
    return { scope, description }
  })
})
const scoped = computed(() => (grants.value ?? []).filter(g => sameScope(g.scope, scope.value)))
const available = computed(() => {
  const level = scope.value.level === 'databasePattern' ? 'database' : scope.value.level
  return props.catalog.filter(p => p.levels.includes(level))
})
const dataNames = ['SELECT', 'INSERT', 'UPDATE', 'DELETE']
const structureNames = new Set([
  'ALTER', 'ALTER ROUTINE', 'CREATE', 'CREATE ROUTINE', 'CREATE TEMPORARY TABLES',
  'CREATE TABLESPACE', 'CREATE VIEW', 'DROP', 'EVENT', 'INDEX', 'REFERENCES', 'SHOW VIEW', 'TRIGGER',
])
const filteredNames = computed(() => {
  const term = filter.value.trim().toUpperCase()
  return available.value.map(p => p.name).filter(name => name.includes(term))
})
const shownData = computed(() => dataNames.filter(name => filteredNames.value.includes(name)))
const privilegeGroups = computed(() => [
  { key: 'data', names: shownData.value },
  { key: 'structure', names: filteredNames.value.filter(name => structureNames.has(name)) },
  { key: 'administration', names: filteredNames.value.filter(name => !dataNames.includes(name) && !structureNames.has(name)) },
].filter(group => group.names.length))
const someShownData = computed(() => shownData.value.some(name => desired.value.includes(name)))
const allShownData = computed(() => shownData.value.length > 0 && shownData.value.every(name => desired.value.includes(name)))
function toggleShownData(checked: boolean) {
  if (busy.value || !editable.value) return
  desired.value = checked
    ? [...new Set([...desired.value, ...shownData.value])]
    : desired.value.filter(name => !shownData.value.includes(name))
}
const current = computed(() => [...new Set(scoped.value.flatMap(g => g.privileges.filter(p => p !== 'USAGE')))])
const currentOption = computed(() => scoped.value.some(g => g.grantOption))
// MySQL reports static and dynamic global privileges separately. Their
// grant options can apply to different sets, even at the same scope.
const separateOptions = computed(() => scope.value.level === 'global' && scoped.value.length > 1)
const delegatedNames = computed(() => scoped.value.filter(g => g.grantOption).flatMap(g => g.privileges))
const scopeReadOnly = computed(() => (scope.value.level === 'databasePattern' && !props.databasePatterns)
  || scoped.value.some(g => !g.editable)
  || current.value.some(name => !available.value.some(p => p.name === name)))
const editable = computed(() => grants.value !== null && props.catalog.length > 0 && !scopeReadOnly.value)
const added = computed(() => desired.value.filter(p => !current.value.includes(p)))
const removed = computed(() => current.value.filter(p => !desired.value.includes(p)))
const addOption = computed(() => grantOption.value && !currentOption.value)
const removeOption = computed(() => !grantOption.value && currentOption.value)
const dirty = computed(() => editable.value && !!(added.value.length || removed.value.length || addOption.value || removeOption.value))
const validDraft = computed(() => {
  if (draftLevel.value === 'databasePattern') return props.databasePatterns && validGrantPattern(draftPattern.value)
  if (draftLevel.value === 'global') return true
  return draftDatabase.value !== '' && (draftLevel.value !== 'table' || draftTable.value !== '')
})
const workspaceDirty = computed(() => dirty.value || tablePatternDirty.value)
watch(workspaceDirty, value => { emit('dirty', value); if (value) saved.value = false }, { flush: 'sync' })
watch(busy, value => emit('busy', value), { flush: 'sync' })

function reset() {
  desired.value = [...current.value]
  grantOption.value = currentOption.value
}
function canDiscard() { return !workspaceDirty.value || window.confirm(t('admin.discardGrants')) }
function useUsernamePattern() {
  draftLevel.value = 'databasePattern'
  draftPattern.value = usernamePattern.value
}
function openDraftScope() {
  if (!validDraft.value) return
  openScope({
    level: draftLevel.value,
    ...(draftLevel.value !== 'global' && { database: draftLevel.value === 'databasePattern' ? draftPattern.value : draftDatabase.value }),
    ...(draftLevel.value === 'table' && { table: draftTable.value }),
  })
}
function openScope(target: Scope) {
  if (!canDiscard()) return
  tablePatternDirty.value = false
  scope.value = target
  reset()
  filter.value = ''
  saved.value = false
  error.value = ''
  choosing.value = false
}
async function chooseScope() {
  if (!canDiscard()) return
  reset()
  tablePatternDirty.value = false
  choosing.value = true
  saved.value = false
  draftLevel.value = 'database'
  draftDatabase.value = scope.value.level === 'databasePattern' ? '' : scope.value.database ?? ''
  draftPattern.value = scope.value.level === 'databasePattern' ? scope.value.database ?? '' : ''
  draftTable.value = ''
  try {
    const result = await api<{ databases: { name: string }[] }>('GET', '/api/databases')
    databases.value = result.databases.map(db => db.name)
  } catch { databases.value = [] } // Exact names also work when metadata is unavailable.
}
watch(draftDatabase, () => { draftTable.value = '' })
watch([draftLevel, draftDatabase, databases, choosing], async ([level, database, names, open], _, onCleanup) => {
  let stale = false
  onCleanup(() => { stale = true })
  tables.value = []
  if (!open || level !== 'table' || !names.includes(database)) return
  try {
    const result = await api<{ tables: { name: string }[] }>('GET', '/api/databases/' + encodeURIComponent(database) + '/tables')
    if (!stale) tables.value = result.tables.map(table => table.name)
  } catch { /* Manual entry remains available. */ }
})

async function readGrants() {
  const result = await api<{ grants: Grant[] }>('GET', props.path)
  grants.value = result.grants
  reset()
}
async function refresh() {
  // The table matcher keeps its own draft and batch results across readbacks.
  if (requestBusy.value || (scope.value.level !== 'tablePattern' && !canDiscard())) return
  requestBusy.value = true
  error.value = ''
  saved.value = false
  try { await readGrants() }
  catch (e) { grants.value = null; error.value = e instanceof Error ? e.message : String(e) }
  finally { requestBusy.value = false }
}
async function save() {
  if (busy.value || !dirty.value || !editable.value) return
  requestBusy.value = true
  error.value = ''
  saved.value = false
  try {
    const scopeValue = scope.value
    if (removed.value.length || removeOption.value) {
      await api('DELETE', props.path, { scope: scopeValue, privileges: removed.value, grantOption: removeOption.value })
    }
    if (added.value.length || addOption.value) {
      // Dynamic global privileges have individual delegation flags. Forward
      // only an explicit change to GRANT OPTION from the draft.
      await api('POST', props.path, { scope: scopeValue, privileges: added.value, grantOption: addOption.value })
    }
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  // Writes can partly succeed. Always read back the server before another edit.
  try { await readGrants(); saved.value = !error.value }
  catch (e) {
    grants.value = null
    error.value = [error.value, e instanceof Error ? e.message : String(e)].filter(Boolean).join(' · ')
  } finally { requestBusy.value = false }
}
onMounted(async () => {
  await refresh()
  if (props.newAccount && grants.value) await chooseScope()
})
</script>

<style scoped>
.grant-workspace, .scope-picker label, .pending, .scope-list button, .unknown { display: flex; flex-direction: column; gap: 5px; }
.grant-workspace { gap: 14px; min-width: 0; }
.section-header, .scope-picker, .actions { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
.section-header { justify-content: space-between; }
h4, p { margin: 0; }
h4 { font-size: 1rem; overflow-wrap: anywhere; }
.scope-list { list-style: none; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(240px, 100%), 1fr)); padding: 0; margin: 0; gap: 7px; }
.scope-list button, .unknown { width: 100%; text-align: start; background: transparent; color: inherit; border: 1px solid v-bind('theme.borderColor'); border-radius: 5px; padding: 8px 12px; font: inherit; overflow-wrap: anywhere; }
.scope-list button { cursor: pointer; }
.scope-list button[aria-pressed=true] { border-color: v-bind('theme.primaryColor'); background: color-mix(in srgb, v-bind('theme.primaryColor') 8%, transparent); }
.scope-list span { font-size: 0.86rem; }
.scope-description { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
.readonly code { display: block; overflow-wrap: anywhere; }
.scope-picker { align-items: end; }
.picker-help { flex-basis: 100%; }
.username-pattern { max-width: 100%; height: auto; min-height: var(--n-height); white-space: normal; overflow-wrap: anywhere; }
.username-pattern :deep(.n-button__content) { min-width: 0; display: block; padding: 4px 0; }
input:not([type=checkbox]), select { box-sizing: border-box; min-width: 0; max-width: 100%; padding: 6px 10px; font: inherit; color: v-bind('theme.textColor1'); background: v-bind('theme.inputColor'); border: 1px solid v-bind('theme.borderColor'); border-radius: 4px; }
select option { color: v-bind('theme.textColor1'); background: v-bind('theme.popoverColor'); }
:where(input, select, button):focus-visible { outline: 2px solid v-bind('theme.primaryColor'); outline-offset: 2px; }
.scope-caption { color: v-bind('theme.textColor2'); overflow-wrap: anywhere; }
.privilege-groups { display: grid; gap: 10px; }
.privilege-group { min-width: 0; overflow: hidden; border: 1px solid v-bind('theme.borderColor'); border-radius: 5px; }
.group-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 4px 12px; padding: 6px 12px; background: v-bind('theme.tableHeaderColor'); color: v-bind('theme.textColor1'); }
.group-heading h5 { margin: 0; font-size: 0.94rem; }
.group-heading .check { font-size: 0.86rem; }
.privilege-matrix { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(190px, 100%), 1fr)); gap: 1px 12px; padding: 6px 12px; }
.privilege-matrix label, .check { display: flex; align-items: center; gap: 7px; padding: 3px 0; cursor: pointer; }
input[type=checkbox] { appearance: none; display: inline-grid; place-content: center; width: 16px; height: 16px; margin: 0; flex: 0 0 auto; border: 1px solid v-bind('theme.iconColor'); border-radius: 3px; background: v-bind('theme.inputColor'); cursor: inherit; }
input[type=checkbox]:is(:checked, :indeterminate) { background: v-bind('theme.primaryColor'); border-color: v-bind('theme.primaryColor'); }
input[type=checkbox]:checked::after { content: ''; width: 4px; height: 8px; border: solid v-bind('theme.baseColor'); border-width: 0 2px 2px 0; transform: translateY(-1px) rotate(45deg); }
input[type=checkbox]:indeterminate::after { content: ''; width: 8px; border-top: 2px solid v-bind('theme.baseColor'); }
input:disabled { opacity: 0.6; }
@media (forced-colors: active) { input[type=checkbox] { appearance: auto; } input[type=checkbox]::after { display: none; } }
.privilege-matrix code { overflow-wrap: anywhere; }
.pending { border-inline-start: 3px solid v-bind('theme.primaryColor'); padding-inline-start: 10px; overflow-wrap: anywhere; }
[role=alert] { color: v-bind('theme.errorColor'); }
</style>

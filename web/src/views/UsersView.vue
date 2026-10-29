<template>
  <section class="admin-view">
    <h2>{{ t('admin.users') }}</h2>
    <p v-if="error" role="alert">{{ error }}</p><p v-if="message" role="status">{{ message }}</p>
    <n-button :disabled="busy" @click="load">{{ t('admin.refresh') }}</n-button>
    <form data-test="create-form" class="toolbar" @submit.prevent="create">
      <label>{{ t('admin.user') }}<input v-model="newUser" data-test="create-user" :required="!createAnonymous" :disabled="createAnonymous" autocomplete="off"></label>
      <label>{{ t('admin.host') }}<input v-model="newHost" data-test="create-host" required autocomplete="off"></label>
      <label>{{ t('admin.password') }}<input v-model="newPassword" data-test="create-password" type="password" autocomplete="new-password"></label>
      <label><input v-model="createAnonymous" data-test="create-anonymous" type="checkbox">{{ t('admin.anonymous') }}</label><n-button attr-type="submit" :disabled="busy">{{ t('admin.create') }}</n-button>
    </form>
    <table><thead><tr><th>{{ t('admin.user') }}</th><th>{{ t('admin.host') }}</th><th>{{ t('admin.plugin') }}</th><th></th></tr></thead><tbody><tr v-for="u in users" :key="`${u.user}@${u.host}`"><td>{{ u.user || t('admin.anonymous') }}</td><td>{{ u.host }}</td><td>{{ u.plugin }}</td><td><n-button data-test="select-user" :disabled="busy" @click="select(u)">{{ t('admin.select') }}</n-button></td></tr></tbody></table>
    <p v-if="!users.length">{{ t('admin.empty') }}</p>
    <section v-if="selected" class="editor">
      <h3>{{ selected.user || t('admin.anonymous') }}@{{ selected.host }}</h3>
      <fieldset :disabled="busy"><legend>{{ t('admin.rename') }}</legend><form class="toolbar" @submit.prevent="rename"><label>{{ t('admin.newUser') }}<input v-model="renameUser" required></label><label>{{ t('admin.newHost') }}<input v-model="renameHost" required></label><n-button attr-type="submit">{{ t('admin.save') }}</n-button></form></fieldset>
      <fieldset :disabled="busy"><legend>{{ t('admin.changePassword') }}</legend><form class="toolbar" @submit.prevent="changePassword"><label>{{ t('admin.password') }}<input v-model="password" type="password" autocomplete="new-password"></label><n-button attr-type="submit">{{ t('admin.save') }}</n-button></form></fieldset>
      <fieldset :disabled="busy"><legend>{{ t('admin.limits') }}</legend><form class="toolbar" @submit.prevent="saveLimits"><label v-for="key in limitKeys" :key="key">{{ t(`admin.${key}`) }}<input v-model.number="limits[key]" type="number" min="0" max="4294967295" required></label><n-button attr-type="submit">{{ t('admin.save') }}</n-button></form></fieldset>
      <fieldset :disabled="busy"><legend>{{ t('admin.privileges') }}</legend>
        <p v-if="grants.some(g => !g.editable)">{{ t('admin.unsupportedGrants') }}</p>
        <div class="toolbar"><label>{{ t('admin.scope') }}<select v-model="level"><option value="global">{{ t('admin.global') }}</option><option value="database">{{ t('admin.database') }}</option><option value="table">{{ t('admin.table') }}</option></select></label><label v-if="level !== 'global'">{{ t('admin.database') }}<input v-model="database"></label><label v-if="level === 'table'">{{ t('admin.table') }}<input v-model="table"></label></div>
        <div class="privilege-matrix"><label v-for="p in availablePrivileges" :key="p.name"><input v-model="desiredPrivileges" :data-test="`privilege-${p.name}`" :disabled="scopeHasUnsupported || !catalog.length" type="checkbox" :value="p.name">{{ p.name }}</label></div>
        <label><input v-model="grantOption" :disabled="scopeHasUnsupported || !catalog.length" type="checkbox">{{ t('admin.grantOption') }}</label>
        <n-button data-test="save-grants" :disabled="!validScope || scopeHasUnsupported || !catalog.length" @click="saveGrants">{{ t('admin.saveGrants') }}</n-button>
        <table><tbody><tr v-for="(g, i) in grants" :key="i"><td>{{ g.scope.level }} {{ g.scope.database }} {{ g.scope.table }}</td><td>{{ g.privileges.join(', ') }}</td><td>{{ g.grantOption ? 'GRANT OPTION' : '' }}</td></tr></tbody></table>
      </fieldset>
      <n-button :disabled="busy" @click="confirmDrop = true">{{ t('admin.drop') }}</n-button>
      <div v-if="confirmDrop" role="alertdialog" :aria-label="t('admin.confirmDrop')"><p>{{ t('admin.confirmDrop') }}: {{ selected.user }}@{{ selected.host }}</p><n-button :disabled="busy" @click="drop">{{ t('admin.confirm') }}</n-button><n-button @click="confirmDrop = false">{{ t('admin.cancel') }}</n-button></div>
    </section>
  </section>
</template>
<script setup lang="ts">
import { NButton } from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
interface Account { user: string; host: string }
interface Limits { queriesPerHour: number; updatesPerHour: number; connectionsPerHour: number; userConnections: number }
interface User extends Account { plugin: string; limits: Limits }
interface Scope { level: string; database?: string; table?: string }
interface Grant { scope: Scope; privileges: string[]; grantOption: boolean; editable: boolean }
interface Privilege { name: string; levels: string[] }
const { t } = useI18n()
const users = ref<User[]>([])
const catalog = ref<Privilege[]>([])
const selected = ref<User | null>(null)
const grants = ref<Grant[]>([])
const busy = ref(false)
const error = ref('')
const message = ref('')
const createAnonymous = ref(false)
const newUser = ref(''); const newHost = ref('%'); const newPassword = ref('')
const renameUser = ref(''); const renameHost = ref(''); const password = ref('')
const confirmDrop = ref(false)
const limitKeys: (keyof Limits)[] = ['queriesPerHour', 'updatesPerHour', 'connectionsPerHour', 'userConnections']
const emptyLimits = (): Limits => ({ queriesPerHour: 0, updatesPerHour: 0, connectionsPerHour: 0, userConnections: 0 })
const limits = ref<Limits>(emptyLimits())
const level = ref('global'); const database = ref(''); const table = ref('')
const desiredPrivileges = ref<string[]>([]); const grantOption = ref(false)
const scope = computed<Scope>(() => ({ level: level.value, ...(level.value !== 'global' ? { database: database.value } : {}), ...(level.value === 'table' ? { table: table.value } : {}) }))
const validScope = computed(() => level.value === 'global' || Boolean(database.value && (level.value !== 'table' || table.value)))
const availablePrivileges = computed(() => catalog.value.filter(p => p.levels.includes(level.value)))
const scopedGrants = computed(() => grants.value.filter(g => g.editable && g.scope.level === scope.value.level && (g.scope.database ?? '') === (scope.value.database ?? '') && (g.scope.table ?? '') === (scope.value.table ?? '')))
const scopeHasUnsupported = computed(() => grants.value.some(g => !g.editable && g.scope.level === scope.value.level && (g.scope.database ?? '') === (scope.value.database ?? '') && (g.scope.table ?? '') === (scope.value.table ?? '')))
const currentPrivileges = computed(() => Array.from(new Set(scopedGrants.value.flatMap(g => g.privileges.includes('ALL PRIVILEGES') ? availablePrivileges.value.map(p => p.name) : g.privileges.filter(p => p !== 'USAGE')))))
const currentGrantOption = computed(() => scopedGrants.value.some(g => g.grantOption))
watch([scope, grants], () => { desiredPrivileges.value = [...currentPrivileges.value]; grantOption.value = currentGrantOption.value }, { deep: true })
function path(suffix = '') {
  const user = selected.value!.user
  return `/api/users/${encodeURIComponent(user || '_anonymous')}${suffix}?host=${encodeURIComponent(selected.value!.host)}${user ? '' : '&anonymous=true'}`
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try { await action(); message.value = t('admin.saved') } catch (e) { error.value = e instanceof Error ? e.message : String(e) } finally { busy.value = false }
}
async function fetchUsers() { users.value = (await api<{ users: User[] }>('GET', '/api/users')).users }
async function fetchGrants() { grants.value = (await api<{ grants: Grant[] }>('GET', path('/grants'))).grants }
async function load() { await run(async () => { await Promise.all([fetchUsers(), api<{ privileges: Privilege[] }>('GET', '/api/users/privileges').then(v => { catalog.value = v.privileges })]) }) }
async function select(u: User) {
  await run(async () => {
    selected.value = u; password.value = ''; renameUser.value = u.user; renameHost.value = u.host; limits.value = { ...u.limits }; confirmDrop.value = false; grants.value = []; level.value = 'global'; database.value = ''; table.value = ''
    await fetchGrants()
  })
}
async function create() { await run(async () => { try { await api('POST', '/api/users', { user: createAnonymous.value ? '' : newUser.value, host: newHost.value, password: newPassword.value }) } finally { newPassword.value = '' }; newUser.value = ''; await fetchUsers() }) }
async function rename() { await run(async () => { const account = { user: renameUser.value, host: renameHost.value }; await api('PATCH', path(), account); selected.value = { ...selected.value!, ...account }; await fetchUsers() }) }
async function changePassword() { await run(async () => { try { await api('PUT', path('/password'), { password: password.value }) } finally { password.value = '' } }) }
async function saveLimits() { await run(async () => { await api('PUT', path('/limits'), limits.value); await fetchUsers() }) }
async function drop() { await run(async () => { await api('DELETE', path()); selected.value = null; confirmDrop.value = false; await fetchUsers() }) }
async function saveGrants() {
  if (scopeHasUnsupported.value || !catalog.value.length) return
  await run(async () => {
    const previous = [...currentPrivileges.value]
    const resetOption = currentGrantOption.value && !grantOption.value
    const removed = resetOption ? previous : previous.filter(p => !desiredPrivileges.value.includes(p))
    if (removed.length || resetOption) await api('DELETE', path('/grants'), { scope: scope.value, privileges: removed, grantOption: resetOption })
    const added = resetOption || grantOption.value ? desiredPrivileges.value : desiredPrivileges.value.filter(p => !previous.includes(p))
    if (added.length || grantOption.value && !currentGrantOption.value && !desiredPrivileges.value.length) await api('POST', path('/grants'), { scope: scope.value, privileges: added, grantOption: grantOption.value })
    await fetchGrants()
  })
}
onMounted(load)
</script>
<style scoped>
.admin-view,.editor{display:flex;flex-direction:column;gap:14px}.toolbar{display:flex;flex-wrap:wrap;gap:12px;align-items:end}label{display:flex;gap:6px;align-items:center}.toolbar label{flex-direction:column;align-items:start}.privilege-matrix{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:10px;margin:14px 0}table{width:100%;border-collapse:collapse;text-align:left}th,td{padding:8px;border-bottom:1px solid #ddd}button{cursor:pointer;padding:6px 12px;align-self:start}input,select{padding:6px;max-width:100%}fieldset{border:1px solid #ccc;border-radius:6px;padding:14px}[role=alert]{color:#bd2424}[role=alertdialog]{padding:12px;border:1px solid #d99b20}
</style>

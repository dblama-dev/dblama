<template>
  <section class="admin-view">
    <header class="section-header">
      <h2>{{ t('admin.users') }}</h2>
      <n-button :disabled="locked" @click="load">{{ t('admin.refresh') }}</n-button>
    </header>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>

    <template v-if="!selected">
      <div class="section-header">
        <input v-model="filter" data-test="find-account" type="search" :aria-label="t('admin.findAccount')" :placeholder="t('admin.findAccount')">
        <n-button v-if="!creating" data-test="start-create" type="primary" :disabled="locked" @click="creating = true">{{ t('admin.create') }}</n-button>
      </div>
      <form v-if="creating" data-test="create-form" class="panel" @submit.prevent="create">
        <h3>{{ t('admin.create') }}</h3>
        <fieldset class="toolbar" :disabled="locked">
          <label>{{ t('admin.user') }}<input v-model="newUser" data-test="create-user" :required="!createAnonymous" :disabled="createAnonymous" autocomplete="off"></label>
          <label>{{ t('admin.host') }}<input v-model="newHost" data-test="create-host" required autocomplete="off" aria-describedby="account-host-help"></label>
          <label>{{ t('admin.password') }}<input v-model="newPassword" data-test="create-password" type="password" autocomplete="new-password"></label>
          <label class="check"><input v-model="createAnonymous" data-test="create-anonymous" type="checkbox">{{ t('admin.anonymous') }}</label>
        </fieldset>
        <p id="account-host-help" class="host-help">{{ t('admin.hostHelp') }}</p>
        <div class="actions">
          <n-button attr-type="submit" type="primary" :disabled="locked">{{ t('admin.create') }}</n-button>
          <n-button :disabled="locked" @click="closeCreate">{{ t('admin.cancel') }}</n-button>
        </div>
      </form>
      <div class="table-wrap">
        <table>
          <thead><tr><th>{{ t('admin.user') }}</th><th>{{ t('admin.host') }}</th><th>{{ t('admin.plugin') }}</th><th></th></tr></thead>
          <tbody><tr v-for="u in filteredUsers" :key="JSON.stringify([u.user, u.host])">
            <td>{{ u.user || t('admin.anonymous') }}</td><td>{{ u.host }}</td><td>{{ u.plugin }}</td>
            <td><n-button data-test="select-user" :disabled="locked" @click="select(u)">{{ t('admin.select') }}</n-button></td>
          </tr></tbody>
        </table>
      </div>
      <p v-if="!filteredUsers.length && !busy">{{ t(filter ? 'admin.noAccountsMatch' : 'admin.empty') }}</p>
    </template>

    <section v-else class="editor">
      <n-button data-test="accounts-list" :disabled="locked" @click="back">{{ t('admin.backToAccounts') }}</n-button>
      <div class="identity">
        <h3>{{ selected.user || t('admin.anonymous') }}@{{ selected.host }}</h3>
        <span>{{ selected.plugin }}</span>
      </div>
      <div class="tabs">
        <n-button v-for="item in tabs" :key="item" :data-test="'tab-' + item" :type="tab === item ? 'primary' : 'default'"
          :aria-pressed="tab === item" :disabled="locked" @click="changeTab(item)">{{ t('admin.' + item) }}</n-button>
      </div>
      <AccountGrants v-if="tab === 'privileges'" :key="grantRevision" :catalog="catalog" :path="path('/grants')" :new-account="justCreated"
        :username="selected.user" :database-patterns="databasePatterns"
        @busy="grantBusy = $event" @dirty="grantDirty = $event"/>
      <div v-else class="settings">
        <fieldset :disabled="locked"><legend>{{ t('admin.rename') }}</legend>
          <form class="toolbar" @submit.prevent="rename">
            <label>{{ t('admin.newUser') }}<input v-model="renameUser" :required="!!selected.user"></label>
            <label>{{ t('admin.newHost') }}<input v-model="renameHost" required></label>
            <n-button attr-type="submit">{{ t('admin.save') }}</n-button>
          </form>
        </fieldset>
        <fieldset :disabled="locked"><legend>{{ t('admin.changePassword') }}</legend>
          <form class="toolbar" @submit.prevent="changePassword">
            <label>{{ t('admin.password') }}<input v-model="password" type="password" autocomplete="new-password"></label>
            <n-button attr-type="submit">{{ t('admin.save') }}</n-button>
          </form>
        </fieldset>
        <fieldset :disabled="locked"><legend>{{ t('admin.limits') }}</legend>
          <form class="toolbar" @submit.prevent="saveLimits">
            <label v-for="key in limitKeys" :key="key">{{ t('admin.' + key) }}
              <input v-model.number="limits[key]" type="number" min="0" max="4294967295" required>
            </label>
            <n-button attr-type="submit">{{ t('admin.save') }}</n-button>
          </form>
        </fieldset>
        <n-button :disabled="locked" type="error" secondary @click="confirmDrop = true">{{ t('admin.drop') }}</n-button>
        <div v-if="confirmDrop" role="alertdialog" :aria-label="t('admin.confirmDrop')" class="panel">
          <p>{{ t('admin.confirmDrop') }} · <strong>{{ selected.user || t('admin.anonymous') }}@{{ selected.host }}</strong></p>
          <div class="actions"><n-button :disabled="locked" type="error" @click="drop">{{ t('admin.confirm') }}</n-button><n-button :disabled="locked" @click="confirmDrop = false">{{ t('admin.cancel') }}</n-button></div>
        </div>
      </div>
    </section>
  </section>
</template>
<script setup lang="ts">
import { NButton } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import AccountGrants from '../components/AccountGrants.vue'
import { useDirtyGuard } from '../lib/useDirtyGuard'

interface Account { user: string; host: string }
interface Limits { queriesPerHour: number; updatesPerHour: number; connectionsPerHour: number; userConnections: number }
interface User extends Account { plugin: string; limits: Limits }
interface Privilege { name: string; levels: string[] }
const { t } = useI18n()
const users = ref<User[]>([])
const catalog = ref<Privilege[]>([])
const databasePatterns = ref(false)
const selected = ref<User | null>(null)
const busy = ref(false)
const grantBusy = ref(false)
const grantDirty = ref(false)
const locked = computed(() => busy.value || grantBusy.value)
const discard = useDirtyGuard(() => grantDirty.value, () => t('admin.discardGrants'))
const grantRevision = ref(0)
const error = ref('')
const message = ref('')
const creating = ref(false)
const justCreated = ref(false)
const createAnonymous = ref(false)
const newUser = ref('')
const newHost = ref('localhost')
const newPassword = ref('')
const renameUser = ref('')
const renameHost = ref('')
const password = ref('')
const confirmDrop = ref(false)
const filter = ref('')
const tabs = ['privileges', 'accountSettings'] as const
const tab = ref<typeof tabs[number]>('privileges')
const limitKeys: (keyof Limits)[] = ['queriesPerHour', 'updatesPerHour', 'connectionsPerHour', 'userConnections']
const limits = ref<Limits>({ queriesPerHour: 0, updatesPerHour: 0, connectionsPerHour: 0, userConnections: 0 })
const filteredUsers = computed(() => {
  const term = filter.value.trim().toLowerCase()
  return users.value.filter(u => ((u.user || t('admin.anonymous')) + '@' + u.host).toLowerCase().includes(term))
})

function path(suffix = '') {
  const { user, host } = selected.value!
  return '/api/users/' + encodeURIComponent(user || '_anonymous') + suffix + '?host=' + encodeURIComponent(host) + (user ? '' : '&anonymous=true')
}
async function run(action: () => Promise<void>, notice = '') {
  if (locked.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try { await action(); if (notice) message.value = t(notice) }
  catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
async function fetchUsers() {
  users.value = (await api<{ users: User[] }>('GET', '/api/users')).users
}
function findAccount(account: Account) {
  return users.value.find(u => u.user === account.user && u.host === account.host)
}
function resetSettings(u: User) {
  renameUser.value = u.user
  renameHost.value = u.host
  limits.value = { ...u.limits }
  password.value = ''
  confirmDrop.value = false
}
async function load() {
  if (!discard()) return
  await run(async () => {
    await fetchUsers()
    if (selected.value) {
      selected.value = findAccount(selected.value) ?? null
      if (selected.value) resetSettings(selected.value)
      grantDirty.value = false
      justCreated.value = false
      grantRevision.value++
    }
    catalog.value = []
    databasePatterns.value = false
    const result = await api<{ privileges: Privilege[]; databasePatterns?: boolean }>('GET', '/api/users/privileges')
    catalog.value = result.privileges
    databasePatterns.value = result.databasePatterns === true
  })
}
function closeCreate() {
  creating.value = false
  newPassword.value = ''
  newUser.value = ''
  newHost.value = 'localhost'
  createAnonymous.value = false
}
function select(u: User, created = false) {
  closeCreate()
  error.value = ''
  message.value = ''
  justCreated.value = created
  selected.value = u
  resetSettings(u)
  tab.value = 'privileges'
}
function back() {
  if (!discard()) return
  grantDirty.value = false
  selected.value = null
  password.value = ''
  error.value = ''
  message.value = ''
}
function changeTab(next: typeof tab.value) {
  if (next === tab.value || !discard()) return
  grantDirty.value = false
  justCreated.value = false
  confirmDrop.value = false
  password.value = ''
  error.value = ''
  message.value = ''
  tab.value = next
}
async function create() {
  await run(async () => {
    const account = { user: createAnonymous.value ? '' : newUser.value, host: newHost.value }
    try { await api('POST', '/api/users', { ...account, password: newPassword.value }) }
    finally { newPassword.value = '' }
    closeCreate()
    await fetchUsers()
    const user = findAccount(account)
    if (user) select(user, true)
  }, 'admin.accountCreated')
}
async function rename() {
  await run(async () => {
    const account = { user: renameUser.value, host: renameHost.value }
    await api('PATCH', path(), account)
    selected.value = { ...selected.value!, ...account }
    await fetchUsers()
  }, 'admin.saved')
}
async function changePassword() {
  await run(async () => {
    try { await api('PUT', path('/password'), { password: password.value }) }
    finally { password.value = '' }
  }, 'admin.saved')
}
async function saveLimits() {
  await run(async () => {
    await api('PUT', path('/limits'), limits.value)
    await fetchUsers()
    selected.value = findAccount(selected.value!) ?? selected.value
  }, 'admin.saved')
}
async function drop() {
  await run(async () => {
    await api('DELETE', path())
    back()
    await fetchUsers()
  }, 'admin.accountDeleted')
}
onMounted(load)
</script>
<style scoped>
.admin-view, .editor, .panel, .settings { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
h2, h3, p { margin: 0; }
.section-header, .toolbar, .actions, .tabs, .identity { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
.section-header { justify-content: space-between; }
.toolbar { align-items: end; }
label { display: flex; flex-direction: column; gap: 6px; }
.check { flex-direction: row; align-items: center; }
.identity h3 { overflow-wrap: anywhere; }
.identity span, .host-help { font-size: 0.89rem; }
.toolbar input, input[type=search] { max-width: 100%; padding: 6px 8px; font: inherit; }
.table-wrap { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; text-align: start; }
th, td { padding: 8px; border-bottom: 1px solid #8886; text-align: start; }
th:last-child, td:last-child { text-align: end; }
fieldset, .panel { border: 1px solid #8886; border-radius: 5px; padding: 14px; min-width: 0; margin: 0; }
.panel fieldset { border: none; padding: 0; }
.editor > button, .settings > button { align-self: start; }
[role=alert] { color: #cb4949; }
</style>

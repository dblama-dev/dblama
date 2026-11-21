<template>
  <section class="admin-view db-page tool-workspace">
    <ToolHeading icon="user" :title="t('admin.users')">
      <ui-button size="small" :disabled="locked" @click="load">
        <template #icon><UiIcon name="refresh" /></template>
        {{ t('admin.refresh') }}
      </ui-button>
      <ui-button
        v-if="!selected && !creating"
        size="small"
        data-test="start-create"
        type="primary"
        :disabled="locked"
        @click="creating = true"
      >
        <template #icon><UiIcon name="plus" /></template>
        {{ t('admin.create') }}
      </ui-button>
    </ToolHeading>
    <p v-if="error && !confirmDrop" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>

    <template v-if="!selected">
      <form
        v-if="creating"
        data-test="create-form"
        class="panel tool-stack tool-card tool-card__body"
        @submit.prevent="create"
      >
        <h3>{{ t('admin.create') }}</h3>
        <fieldset class="toolbar" :disabled="locked">
          <label>
            {{ t('admin.user') }}
            <input
              v-model="newUser"
              data-test="create-user"
              :required="!createAnonymous"
              :disabled="createAnonymous"
              autocomplete="off"
            />
          </label>
          <label>
            {{ t('admin.host') }}
            <input
              v-model="newHost"
              data-test="create-host"
              required
              autocomplete="off"
              aria-describedby="account-host-help"
            />
          </label>
          <label>
            {{ t('admin.password') }}
            <input
              v-model="newPassword"
              data-test="create-password"
              type="password"
              autocomplete="new-password"
            />
          </label>
          <label class="check">
            <input v-model="createAnonymous" data-test="create-anonymous" type="checkbox" />
            {{ t('admin.anonymous') }}
          </label>
        </fieldset>
        <p id="account-host-help" class="host-help">{{ t('admin.hostHelp') }}</p>
        <div class="actions">
          <ui-button attr-type="submit" type="primary" :disabled="locked">{{ t('admin.create') }}</ui-button>
          <ui-button :disabled="locked" @click="closeCreate">{{ t('admin.cancel') }}</ui-button>
        </div>
      </form>
      <section class="tool-card account-list">
        <header class="tool-card__head">
          <label class="account-filter">
            <UiIcon name="search" :size="16" />
            <input
              v-model="filter"
              data-test="find-account"
              type="search"
              :aria-label="t('admin.findAccount')"
              :placeholder="t('admin.findAccount')"
            />
          </label>
        </header>
        <div class="tool-scroll admin-records">
          <table class="tool-grid">
            <thead>
              <tr>
                <th>{{ t('admin.user') }}</th>
                <th>{{ t('admin.host') }}</th>
                <th>{{ t('admin.plugin') }}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="u in filteredUsers" :key="JSON.stringify([u.user, u.host])">
                <td class="account-name"><span>{{ u.user || t('admin.anonymous') }}</span><span class="mobile-host">@{{ u.host }}</span></td>
                <td class="account-host">{{ u.host }}</td>
                <td class="account-plugin">{{ u.plugin }}</td>
                <td class="account-action">
                  <ui-button
                    size="small"
                    quaternary
                    data-test="select-user"
                    :disabled="locked"
                    @click="select(u)"
                  >
                    {{ t('admin.select') }}
                  </ui-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!filteredUsers.length && !busy" class="tool-empty">
          {{ t(filter ? 'admin.noAccountsMatch' : 'admin.empty') }}
        </p>
      </section>
    </template>

    <section v-else class="editor tool-stack">
      <div class="account-top">
        <ui-button quaternary size="small" data-test="accounts-list" :disabled="locked" @click="back">
          <template #icon><UiIcon name="chevron-left" /></template>
          {{ t('admin.backToAccounts') }}
        </ui-button>
        <div class="identity">
          <h3>{{ selected.user || t('admin.anonymous') }}@{{ selected.host }}</h3>
          <span>{{ selected.plugin }}</span>
        </div>
      </div>
      <div class="tabs">
        <ui-button
          v-for="item in tabs"
          :key="item"
          :data-test="'tab-' + item"
          quaternary
          :class="{ active: tab === item }"
          :aria-pressed="tab === item"
          :disabled="locked"
          @click="changeTab(item)"
        >
          {{ t('admin.' + item) }}
        </ui-button>
      </div>
      <AccountGrants
        v-if="tab === 'privileges'"
        :key="grantRevision"
        :catalog="catalog"
        :path="path('/grants')"
        :new-account="justCreated"
        :username="selected.user"
        :database-patterns="databasePatterns"
        @busy="grantBusy = $event"
        @dirty="grantDirty = $event"
      />
      <div v-else class="settings tool-stack">
        <fieldset :disabled="locked">
          <legend>{{ t('admin.rename') }}</legend>
          <form class="toolbar" @submit.prevent="rename">
            <label>
              {{ t('admin.newUser') }}
              <input v-model="renameUser" :required="!!selected.user" />
            </label>
            <label>
              {{ t('admin.newHost') }}
              <input v-model="renameHost" required />
            </label>
            <ui-button attr-type="submit">{{ t('admin.save') }}</ui-button>
          </form>
        </fieldset>
        <fieldset :disabled="locked">
          <legend>{{ t('admin.changePassword') }}</legend>
          <form class="toolbar" @submit.prevent="changePassword">
            <label>
              {{ t('admin.password') }}
              <input v-model="password" type="password" autocomplete="new-password" />
            </label>
            <ui-button attr-type="submit">{{ t('admin.save') }}</ui-button>
          </form>
        </fieldset>
        <fieldset :disabled="locked">
          <legend>{{ t('admin.limits') }}</legend>
          <form class="toolbar limits" @submit.prevent="saveLimits">
            <label v-for="key in limitKeys" :key="key">
              {{ t('admin.' + key) }}
              <input v-model.number="limits[key]" type="number" min="0" max="4294967295" required />
            </label>
            <ui-button attr-type="submit">{{ t('admin.save') }}</ui-button>
          </form>
        </fieldset>
        <ui-button :disabled="locked" type="error" secondary @click="confirmDrop = true">
          {{ t('admin.drop') }}
        </ui-button>
        <UiModal v-model:show="confirmDrop" preset="dialog" :title="t('admin.confirmDrop')"
          :closable="!locked" :close-on-esc="!locked" :mask-closable="!locked">
          <strong>{{ selected.user || t('admin.anonymous') }}@{{ selected.host }}</strong>
          <p v-if="error" role="alert">{{ error }}</p>
          <template #footer>
            <ui-button data-cancel :disabled="locked" @click="confirmDrop = false">{{ t('admin.cancel') }}</ui-button>
            <ui-button :disabled="locked" type="error" @click="drop">{{ t('admin.drop') }}</ui-button>
          </template>
        </UiModal>
      </div>
    </section>
  </section>
</template>
<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { UiButton, UiModal } from '../ui'
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
<style scoped src="./adminRecords.css"></style>
<style scoped>
.account-list {
  overflow: hidden;
}
.account-filter {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--db-muted);
  max-width: 100%;
}
.account-filter input {
  width: min(310px, 65vw);
}
.tool-grid td:not(:last-child) {
  font: 13px var(--db-mono);
  overflow-wrap: anywhere;
}
.tool-grid th:last-child,
.tool-grid td:last-child {
  text-align: end;
}
.account-top {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}
.account-top .identity {
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  align-items: center;
}
.identity h3 {
  font: 600 20px/1.3 var(--db-mono);
  overflow-wrap: anywhere;
}
.identity span {
  font-size: 12px;
  color: var(--db-muted);
}
.tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--db-line);
  flex-wrap: wrap;
}
.tabs .ui-button {
  margin-bottom: -1px;
  border-radius: 0;
}
.tabs .active {
  color: var(--db-accent);
  box-shadow: inset 0 -2px var(--db-accent);
}
.settings fieldset {
  min-width: 0;
  margin: 0;
  border: 1px solid var(--db-line);
  border-radius: 9px;
  padding: 14px;
}
.settings legend {
  padding: 0 5px;
  font-weight: 600;
}
.toolbar,
.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: end;
  gap: 10px 16px;
}
.toolbar label {
  min-width: 0;
  max-width: 100%;
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 13px;
}
.toolbar label.check {
  flex-direction: row;
  align-items: center;
}
.panel fieldset {
  border: 0;
  margin: 0;
  padding: 0;
}
.settings > button {
  align-self: start;
}
.host-help {
  color: var(--db-muted);
  font-size: 12px;
}
[role="alert"] {
  color: var(--db-danger, #b95959);
  overflow-wrap: anywhere;
}
.mobile-host { display: none; }
@container records (max-width: 660px) {
  .account-list tbody tr {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 3px 10px;
    align-items: center;
  }
  .account-list .account-name { font-weight: 600; }
  .account-name .mobile-host { display: inline; color: var(--db-muted); font-weight: normal; }
  .account-host { display: none; }
  .account-list .account-plugin { color: var(--db-muted); font-size: 12px; grid-column: 1; }
  .account-action { grid-column: 2; grid-row: 1 / 3; }
}
@media (max-width: 640px) {
  .toolbar { display: grid; grid-template-columns: minmax(0, 1fr); align-items: end; gap: 10px; }
  .toolbar.limits { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .toolbar button { justify-self: start; }
  .account-filter { width: 100%; }
  .account-filter input { flex: 1; }
}
</style>

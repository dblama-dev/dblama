<template>
  <section class="db-page tool-workspace">
    <ToolHeading icon="user" :title="t('roles.title')">
      <UiButton :disabled="busy" @click="refresh">{{ t('admin.refresh') }}</UiButton>
      <UiButton :disabled="busy" type="primary" @click="startCreate">{{ t('roles.create') }}</UiButton>
    </ToolHeading>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="notice" role="status">{{ t('admin.saved') }}</p>
    <fieldset class="role-editor tool-stack" :disabled="busy">
      <form v-if="creating" class="tool-card tool-card__body tool-stack" @submit.prevent="createRole">
        <h3>{{ t('roles.create') }}</h3>
        <label>{{ t('admin.name') }} <input v-model="name" required autocomplete="off"></label>
        <label><input v-model="login" type="checkbox"> {{ t('roles.login') }}</label>
        <label>{{ t('admin.password') }} <input v-model="password" type="password" autocomplete="new-password"></label>
        <div class="role-actions">
          <UiButton type="primary" attr-type="submit">{{ t('roles.create') }}</UiButton>
          <UiButton @click="closeCreate">{{ t('admin.cancel') }}</UiButton>
        </div>
      </form>
      <template v-else>
        <label>{{ t('roles.role') }}
          <select v-model="selection" @change="resetEditor">
            <option value="" disabled>{{ t('roles.choose') }}</option>
            <option v-for="r in roles" :key="r.name" :value="r.name">{{ r.name }}</option>
          </select>
        </label>
        <template v-if="role">
          <section class="tool-card tool-card__body tool-stack">
            <h3>{{ t('roles.settings') }} · {{ role.name }}</h3>
            <p v-if="role.superuser">{{ t('roles.superuser') }}</p>
            <div class="role-actions">
              <UiButton @click="change({ action: 'login', login: !role.login })">{{ t(role.login ? 'roles.disable' : 'roles.enable') }}</UiButton>
              <UiButton type="error" secondary @click="confirmDrop = true">{{ t('roles.drop') }}</UiButton>
            </div>
            <form class="role-actions" @submit.prevent="change({ action: 'password', password })">
              <label>{{ t('admin.password') }} <input v-model="password" required type="password" autocomplete="new-password"></label>
              <UiButton attr-type="submit">{{ t('admin.changePassword') }}</UiButton>
            </form>
          </section>
          <section class="tool-card tool-card__body tool-stack">
            <h3>{{ t('roles.memberships') }}</h3>
            <div v-for="parent in role.memberships" :key="parent" class="role-actions">
              <strong>{{ parent }}</strong>
              <UiButton size="small" @click="change({ action: 'leave', group: parent })">{{ t('roles.leave') }}</UiButton>
            </div>
            <form class="role-actions" @submit.prevent="change({ action: 'join', group })">
              <label>{{ t('roles.group') }}
                <select v-model="group" required>
                  <option value="" disabled>{{ t('roles.choose') }}</option>
                  <option v-for="r in availableGroups" :key="r.name" :value="r.name">{{ r.name }}</option>
                </select>
              </label>
              <UiButton attr-type="submit" :disabled="!group">{{ t('roles.join') }}</UiButton>
            </form>
          </section>
        </template>
      </template>
    </fieldset>
    <RolePrivileges v-if="role && !creating" :key="role.name" :role="role.name" :revision="revision" />
    <UiModal v-model:show="confirmDrop" preset="dialog" :title="t('roles.confirmDrop', { name: selection })">
      <template #footer>
        <UiButton @click="confirmDrop = false">{{ t('admin.cancel') }}</UiButton>
        <UiButton type="error" @click="confirmDrop = false; change({ action: 'drop' })">{{ t('roles.drop') }}</UiButton>
      </template>
    </UiModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import ToolHeading from '../components/ToolHeading.vue'
import RolePrivileges from '../components/RolePrivileges.vue'
import { UiButton, UiModal } from '../ui'

interface Role { name: string; login: boolean; superuser: boolean; memberships: string[] }
const { t } = useI18n()
const session = useSessionStore()
const roles = ref<Role[]>([])
const selection = ref('')
const role = computed(() => roles.value.find(r => r.name === selection.value))
const availableGroups = computed(() => roles.value.filter(r => r.name !== selection.value && !role.value?.memberships.includes(r.name)))
const name = ref('')
const group = ref('')
const password = ref('')
const login = ref(false)
const creating = ref(false)
const confirmDrop = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref(false)
const revision = ref(0)

function resetEditor() {
  password.value = group.value = error.value = ''
  notice.value = false
}
function startCreate() {
  resetEditor()
  name.value = ''
  login.value = false
  creating.value = true
}
function closeCreate() {
  resetEditor()
  creating.value = false
}
async function load() {
  roles.value = await api<Role[]>('GET', '/api/roles')
  if (!role.value) selection.value = roles.value.find(r => r.name === session.sessionInfo?.driver.user)?.name ?? ''
  revision.value++
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = false
  try { await action() }
  catch (err) { error.value = err instanceof Error ? err.message : String(err) }
  finally { busy.value = false }
}
function refresh() { return run(load) }
function change(fields: { action: string; password?: string; login?: boolean; group?: string }) {
  return run(async () => {
    try { await api('POST', '/api/roles', { name: selection.value, ...fields }) }
    finally { password.value = '' }
    group.value = ''
    await load()
    notice.value = true
  })
}
function createRole() {
  return run(async () => {
    try { await api('POST', '/api/roles', { action: 'create', name: name.value, login: login.value, password: password.value }) }
    finally { password.value = '' }
    selection.value = name.value
    closeCreate()
    await load()
    notice.value = true
  })
}
onMounted(refresh)
</script>

<style scoped>
.role-editor { border: 0; padding: 0; margin: 0; min-width: 0; }
.role-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
label, input, select { max-width: 100%; }
[role="alert"] { color: var(--db-danger); }
</style>

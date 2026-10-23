<template>
  <main class="login-view">
    <n-card class="login-view__card" :title="t('login.title')">
      <n-form :model="form" label-placement="top" @submit.prevent="submit">
        <n-form-item :label="t('login.driver')">
          <n-select v-model:value="form.driver" :options="driverOptions" />
        </n-form-item>
        <n-form-item :label="t('login.host')">
          <n-input v-model:value="form.host" :placeholder="t('login.host')" />
        </n-form-item>
        <n-form-item :label="t('login.port')">
          <n-input v-model:value="form.port" :placeholder="portPlaceholder" />
        </n-form-item>
        <n-form-item :label="t('login.user')">
          <n-input v-model:value="form.user" :placeholder="t('login.user')" />
        </n-form-item>
        <n-form-item :label="t('login.password')">
          <n-input v-model:value="form.password" type="password" :placeholder="t('login.password')" />
        </n-form-item>
        <n-form-item :label="t('login.database')">
          <n-input v-model:value="form.database" :placeholder="t('login.databaseOptional')" />
        </n-form-item>
        <n-button
          attr-type="submit"
          type="primary"
          block
          :loading="submitting"
          :disabled="!canSubmit"
        >
          {{ t('login.submit') }}
        </n-button>
      </n-form>
    </n-card>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NCard, NForm, NFormItem, NInput, NSelect, useNotification } from 'naive-ui'

import { ApiError } from '../api/client'
import { useSessionStore } from '../stores/session'

/** localStorage key for the last successful login (driver, host, user —
 * never the password). */
const LAST_LOGIN_KEY = 'dbm-last-login'

const { t } = useI18n()
const router = useRouter()
const store = useSessionStore()
const notification = useNotification()

const form = reactive({
  driver: null as string | null,
  host: '',
  port: '',
  user: '',
  password: '',
  database: '',
})
const submitting = ref(false)

const driverOptions = computed(() => store.drivers.map((d) => ({ label: d.name, value: d.id })))

// Port stays empty by design: the server falls back to the driver's
// defaultPort, so the placeholder advertises it instead.
const portPlaceholder = computed(
  () => store.drivers.find((d) => d.id === form.driver)?.defaultPort ?? '',
)

const canSubmit = computed(
  () => form.driver !== null && form.host !== '' && form.user !== '' && form.password !== '',
)

onMounted(() => {
  restoreLastLogin()
  void store.loadDrivers().catch((e: unknown) => {
    notification.error({
      title: t('login.failed'),
      content: e instanceof ApiError ? e.message : String(e),
    })
  })
})

/** Prefills driver/host/user from the last successful login. */
function restoreLastLogin(): void {
  try {
    const saved = JSON.parse(localStorage.getItem(LAST_LOGIN_KEY) ?? '{}') as Partial<{
      driver: string
      host: string
      user: string
    }>
    form.driver = saved.driver ?? null
    form.host = saved.host ?? ''
    form.user = saved.user ?? ''
  } catch {
    // Malformed persisted value: start with a clean form.
  }
}

async function submit(): Promise<void> {
  if (submitting.value) return
  const { driver, host, port, user, password, database } = form
  if (driver === null || host === '' || user === '' || password === '') return
  submitting.value = true
  try {
    await store.login({ driver, host, port, user, password, database: database === '' ? undefined : database })
    localStorage.setItem(LAST_LOGIN_KEY, JSON.stringify({ driver, host, user }))
    await router.push('/server')
  } catch (e) {
    notification.error({
      title: t('login.failed'),
      content: e instanceof ApiError ? e.message : String(e),
    })
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.login-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 16px;
}

.login-view__card {
  width: 380px;
}
</style>

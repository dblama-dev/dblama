<template>
  <main class="login-view">
    <header class="login-brand"><span class="db-brand-mark"><UiIcon name="stack" :size="23" /></span>DB Manager</header>
    <div class="login-stage">
      <section class="login-card db-panel">
        <header class="login-card__head">
          <span class="db-heading-icon"><UiIcon name="server" :size="22" /></span>
          <h1>{{ t('workspace.connect') }}</h1>
        </header>
        <form @submit.prevent="submit">
          <div class="login-field"><label for="login-driver">{{ t('login.driver') }}</label>
            <ui-select v-model:value="form.driver" filterable :aria-label="t('login.driver')" :input-props="{ id: 'login-driver', 'aria-label': t('login.driver') }" :options="driverOptions" />
          </div>
          <div class="login-address">
            <div class="login-field"><label for="login-host">{{ t('login.host') }}</label>
              <ui-input v-model:value="form.host" :input-props="{ id:'login-host', autocapitalize:'none', spellcheck:false }" :placeholder="t('workspace.host')" />
            </div>
            <div class="login-field"><label for="login-port">{{ t('login.port') }}</label>
              <ui-input v-model:value="form.port" :input-props="{ id:'login-port', inputmode:'numeric' }" :placeholder="portPlaceholder" />
            </div>
          </div>
          <div class="login-field"><label for="login-user">{{ t('login.user') }}</label>
            <ui-input v-model:value="form.user" :input-props="{ id:'login-user', autocapitalize:'none', spellcheck:false }" autocomplete="username" :placeholder="t('login.user')" />
          </div>
          <div class="login-field"><label for="login-password">{{ t('login.password') }}</label>
            <ui-input v-model:value="form.password" :input-props="{ id:'login-password' }" type="password" show-password-on="click" autocomplete="current-password" :placeholder="t('login.password')" />
          </div>
          <div class="login-field"><label for="login-db">{{ t('login.database') }}</label>
            <ui-input v-model:value="form.database" :input-props="{ id:'login-db', autocapitalize:'none', spellcheck:false }" :placeholder="t('login.databaseOptional')" />
          </div>
          <ui-button attr-type="submit" type="primary" block :loading="submitting" :disabled="!canSubmit">
            {{ t('login.submit') }}<UiIcon name="arrow-right" />
          </ui-button>
        </form>
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { UiButton, UiInput, UiSelect, useNotification } from '../ui'

import UiIcon from '../components/UiIcon.vue'
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
.login-view { display: flex; flex-direction: column; min-height: 100dvh; color: var(--db-ink); background-color: var(--db-sidebar); background-image: radial-gradient(color-mix(in srgb,var(--db-line) 78%,transparent) .85px,transparent .85px); background-size: 17px 17px; }
.login-brand { display: flex; align-items: center; gap: 11px; padding: 15px clamp(16px,3vw,42px); background: var(--db-panel); border-bottom: 1px solid var(--db-line); font-size: 20px; font-weight: 650; letter-spacing: -.045em; }
.login-stage { flex: 1; display: grid; place-items: center; padding: 28px 16px; }
.login-card { width: min(100%,448px); overflow: hidden; box-shadow: 0 8px 32px #253a6b0a; }
.login-card__head { padding: 19px 25px; display: flex; align-items: center; gap: 12px; border-bottom: 1px solid var(--db-line); background: linear-gradient(105deg,var(--db-panel),var(--db-tint)); }
.login-card h1 { font-size: clamp(21px,4.5vw,25px); line-height: 1.25; font-weight: 650; letter-spacing: -.04em; margin: 0; }
.login-card form { padding: 20px 25px 24px; }
.login-address { display: grid; grid-template-columns: minmax(0,1fr) 85px; gap: 12px; }
.login-card form, .login-field { display:grid; gap:14px; }
.login-field { gap:5px; align-content:start; min-width:0; }
.login-field label { font-size:13px; font-weight:550; }
@media(max-width: 440px) { .login-card form,.login-card__head { padding-inline: 17px; } }
</style>

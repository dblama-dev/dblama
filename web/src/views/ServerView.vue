<template>
  <section class="server-view db-page">
    <CatalogHeader :scope="t('server.title')" icon="server" :title="driver?.host || t('sideNavigation.server')"
      :engine="driver ? driver.name + ' ' + driver.version : undefined" :detail="driver?.port ? t('login.port') + ' ' + driver.port : undefined" :stats="stats" />
    <ui-alert v-if="error" type="error" :title="t('server.loadFailed')">{{ error }}</ui-alert>
    <DatabaseActions @changed="onDatabaseChanged">
      <template #heading><h2 class="db-section-title">{{ t('workspace.databases') }}</h2></template>
    </DatabaseActions>
    <DatabaseTable :databases="databases" :loading="loading" @refresh="load" @open="openDb" />
    <nav v-if="caps?.users || caps?.monitor" class="db-toolbar" :aria-label="t('sideNavigation.server')">
      <ui-button v-if="caps.users" tag="a" href="/server/users" size="small" @click.exact.prevent="router.push('/server/users')">
        <UiIcon name="user" />{{ t('admin.users') }}
      </ui-button>
      <ui-button v-if="caps.monitor" tag="a" href="/server/processes" size="small" @click.exact.prevent="router.push('/server/processes')">
        <UiIcon name="activity" />{{ t('admin.monitor') }}
      </ui-button>
    </nav>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { UiAlert, UiButton } from '../ui'

import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import type { DatabaseInfo } from '../api/types'
import UiIcon from '../components/UiIcon.vue'
import CatalogHeader from '../components/CatalogHeader.vue'
import { formatBytes } from '../lib/format'
import DatabaseActions from '../components/DatabaseActions.vue'
import { useCatalogStore } from '../stores/catalog'
import DatabaseTable from '../components/DatabaseTable.vue'

const session = useSessionStore()
const caps = computed(() => session.sessionInfo?.caps)
const { t } = useI18n()
const router = useRouter()

const databases = ref<DatabaseInfo[]>([])
const loading = ref(false)
const error = ref('')

/** Fetches the database list; the refresh button re-runs it. */
async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const payload = await api<{ databases: DatabaseInfo[] }>('GET', '/api/databases')
    databases.value = payload.databases ?? []
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

const driver = computed(() => session.sessionInfo?.driver)
const stats = computed(() => [
  { icon: 'db', label: t('workspace.databases'), value: error.value || loading.value ? '—' : String(databases.value.length) },
  { icon: 'disk', label: t('workspace.storage'), value: error.value || loading.value ? '—' : formatBytes(databases.value.reduce((n, db) => n + db.sizeBytes, 0)) },
  { icon: 'user', label: t('workspace.currentUser'), value: driver.value?.user || '—' },
])

async function onDatabaseChanged(): Promise<void> {
  useCatalogStore().databaseRevision++
  await load()
}
onMounted(load)

/** Opens the database tree view for the clicked row. */
function openDb(name: string): void {
  void router.push(`/db/${encodeURIComponent(name)}`)
}
</script>

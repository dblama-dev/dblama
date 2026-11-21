<template>
  <section class="server-view db-page">
    <CatalogHeader :scope="t('server.title')" icon="server" :title="driver?.host || t('sideNavigation.server')"
      :engine="driver ? driver.name + ' ' + driver.version : undefined" :detail="driver?.port ? t('login.port') + ' ' + driver.port : undefined" :stats="stats" />
    <ui-alert v-if="error" type="error" :title="t('server.loadFailed')">{{ error }}</ui-alert>
    <DatabaseActions>
      <template #heading><h2 class="db-section-title">{{ t('workspace.databases') }}</h2></template>
    </DatabaseActions>
    <DatabaseTable :databases="databases" :loading="loading" @refresh="load" @open="openDb" />
    <nav v-if="tools.length" class="server-tools" aria-labelledby="server-management">
      <h2 id="server-management" class="db-section-title">{{ t('server.management') }}</h2>
      <div class="db-toolbar">
        <ui-button v-for="tool in tools" :key="tool.path" tag="a" :href="tool.path" size="small" @click.exact.prevent="router.push(tool.path)">
          <UiIcon :name="tool.icon" />{{ t(tool.label) }}
        </ui-button>
      </div>
    </nav>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { UiAlert, UiButton } from '../ui'

import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import type { DatabaseInfo } from '../api/types'
import UiIcon from '../components/UiIcon.vue'
import CatalogHeader from '../components/CatalogHeader.vue'
import { formatBytes } from '../lib/format'
import { serverDestinations } from '../lib/workspaceNavigation'
import DatabaseActions from '../components/DatabaseActions.vue'
import { useCatalogStore } from '../stores/catalog'
import DatabaseTable from '../components/DatabaseTable.vue'

const session = useSessionStore()
const tools = computed(() => serverDestinations(session.sessionInfo?.caps))
const { t } = useI18n()
const router = useRouter()
const catalog = useCatalogStore()

const databases = ref<DatabaseInfo[]>([])
const loading = ref(false)
const error = ref('')
let loadID = 0

/** Fetches the database list; the refresh button re-runs it. */
async function load(): Promise<void> {
  const id = ++loadID
  loading.value = true
  error.value = ''
  try {
    const payload = await api<{ databases: DatabaseInfo[] }>('GET', '/api/databases')
    if (id === loadID) databases.value = payload.databases ?? []
  } catch (e) {
    if (id === loadID) error.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (id === loadID) loading.value = false
  }
}

const driver = computed(() => session.sessionInfo?.driver)
const stats = computed(() => [
  { icon: 'db', label: t('workspace.databases'), value: error.value || loading.value ? '—' : String(databases.value.length) },
  { icon: 'disk', label: t('workspace.storage'), value: error.value || loading.value ? '—' : formatBytes(databases.value.reduce((n, db) => n + db.sizeBytes, 0)) },
  { icon: 'user', label: t('workspace.currentUser'), value: driver.value?.user || '—' },
])

watch(() => catalog.databaseRevision, load, { immediate: true })

/** Opens the database tree view for the clicked row. */
function openDb(name: string): void {
  void router.push(`/db/${encodeURIComponent(name)}`)
}
</script>
<style scoped>
.server-tools { display: grid; gap: 10px; }
</style>

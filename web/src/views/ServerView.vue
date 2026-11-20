<template>
  <section class="server-view">
    <header class="server-view__header">
      <n-h2>{{ t('server.title') }}</n-h2>
      <nav v-if="caps?.users || caps?.monitor" :aria-label="t('server.title')">
        <n-button v-if="caps.users" tag="a" href="/server/users" size="small" @click.exact.prevent="router.push('/server/users')">{{ t('admin.users') }}</n-button>
        <n-button v-if="caps.monitor" tag="a" href="/server/processes" size="small" @click.exact.prevent="router.push('/server/processes')">{{ t('admin.monitor') }}</n-button>
      </nav>
    </header>
    <n-alert v-if="error" type="error" :title="t('server.loadFailed')">{{ error }}</n-alert>
    <DatabaseActions @changed="onDatabaseChanged" />
    <DatabaseTable :databases="databases" :loading="loading" @refresh="load" @open="openDb" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NH2 } from 'naive-ui'

import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import type { DatabaseInfo } from '../api/types'
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

<style scoped>
.server-view__header, .server-view__header nav { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.server-view__header { justify-content: space-between; }
.server-view__header h2 { margin: 0; }
.server-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>

<template>
  <section class="server-view">
    <n-h2>{{ t('server.title') }}</n-h2>
    <n-alert v-if="error" type="error" :title="t('server.loadFailed')">{{ error }}</n-alert>
    <DatabaseTable :databases="databases" :loading="loading" @refresh="load" />
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NH2 } from 'naive-ui'

import { api } from '../api/client'
import type { DatabaseInfo } from '../api/types'
import DatabaseTable from '../components/DatabaseTable.vue'

const { t } = useI18n()

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

onMounted(load)
</script>

<style scoped>
.server-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>

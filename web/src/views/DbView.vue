<template>
  <section class="db-view">
    <h2>{{ db }}</h2>
    <DatabaseActions :db="db" @changed="changed" @renamed="renamed" @dropped="dropped" />
    <n-alert v-if="catalog.error" type="error">{{ catalog.error }}</n-alert>
    <n-space>
      <n-button size="small" :loading="catalog.loading" @click="load">{{ t('server.refresh') }}</n-button>
      <n-button size="small" @click="newTable">{{ t('tree.newTable') }}</n-button>
    </n-space>
    <TableTransfer :db="db" :tables="selected" @changed="load" />
    <n-table size="small">
      <thead><tr><th></th><th>{{ t('server.columns.name') }}</th><th>{{ t('grid.total') }}</th><th>{{ t('server.columns.size') }}</th></tr></thead>
      <tbody>
        <tr v-for="table in catalog.tables" :key="table.name">
          <td><input v-if="!table.isView" v-model="selected" type="checkbox" :value="table.name" :aria-label="table.name" /></td>
          <td><n-button text @click="openTable(table.name)">{{ table.name }}</n-button></td>
          <td>{{ table.rows }}</td><td>{{ formatBytes(table.sizeBytes) }}</td>
        </tr>
      </tbody>
    </n-table>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NSpace, NTable } from 'naive-ui'
import DatabaseActions from '../components/DatabaseActions.vue'
import TableTransfer from '../components/TableTransfer.vue'
import { useCatalogStore } from '../stores/catalog'
import { formatBytes } from '../lib/format'
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const catalog = useCatalogStore()
const db = computed(() => String(route.params.db ?? ''))
const selected = ref<string[]>([])
async function load(): Promise<void> { selected.value=[]; await catalog.fetchTables(db.value) }
async function changed(): Promise<void> { catalog.databaseRevision++; await load() }
function renamed(name:string): void { void router.replace(catalog.path(`/db/${encodeURIComponent(name)}`)) }
function dropped(): void { void router.replace('/server') }
function openTable(name:string): void { void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent(name)}`)) }
function newTable(): void { void router.push(catalog.path(`/db/${encodeURIComponent(db.value)}/table/_new?view=structure`)) }
watch([db,()=>catalog.schema],load,{immediate:true})
</script>
<style scoped>
.db-view { display:flex; flex-direction:column; gap:16px; }
</style>

<template>
  <section class="admin-view">
    <h2>{{ t('admin.monitor') }}</h2>
    <p v-if="error" role="alert">{{ error }}</p>
    <div class="toolbar">
      <button data-test="refresh" :disabled="loading" @click="load">{{ t('admin.refresh') }}</button>
      <label><input v-model="autoRefresh" data-test="auto-refresh" type="checkbox">{{ t('admin.autoRefresh') }}</label>
      <span v-if="loading" role="status">{{ t('admin.loading') }}</span>
    </div>
    <div v-if="killTarget !== null" class="confirmation" role="alertdialog" :aria-label="t('admin.kill')">
      <p>{{ t('admin.confirmKill', { id: killTarget }) }}</p>
      <button data-test="confirm-kill" :disabled="loading" @click="kill">{{ t('admin.confirm') }}</button>
      <button @click="killTarget = null">{{ t('admin.cancel') }}</button>
    </div>
    <h3>{{ t('admin.processes') }}</h3>
    <div class="table-scroll"><table>
      <thead><tr><th>ID</th><th>{{ t('admin.user') }}</th><th>{{ t('admin.host') }}</th><th>{{ t('admin.database') }}</th><th>{{ t('admin.command') }}</th><th>{{ t('admin.seconds') }}</th><th>{{ t('admin.state') }}</th><th>{{ t('admin.query') }}</th><th></th></tr></thead>
      <tbody><tr v-for="p in processes" :key="p.id"><td>{{ p.id }}</td><td>{{ p.user }}</td><td>{{ p.host }}</td><td>{{ p.database }}</td><td>{{ p.command }}</td><td>{{ p.seconds }}</td><td>{{ p.state }}</td><td><pre>{{ p.query }}</pre></td><td><button data-test="kill" :disabled="loading" @click="killTarget = p.id">{{ t('admin.kill') }}</button></td></tr></tbody>
    </table></div>
    <p v-if="!processes.length">{{ t('admin.empty') }}</p>
    <label>{{ t('admin.filter') }}<input v-model="filter" type="search"></label>
    <section v-for="group in groups" :key="group.key">
      <h3>{{ t(`admin.${group.key}`) }}</h3>
      <div class="table-scroll"><table><thead><tr><th>{{ t('admin.name') }}</th><th>{{ t('admin.value') }}</th></tr></thead><tbody><tr v-for="v in group.values.filter(matches)" :key="v.name"><td>{{ v.name }}</td><td>{{ v.value }}</td></tr></tbody></table></div>
    </section>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
interface Process { id: number; user: string; host: string; database: string; command: string; seconds: number; state: string; query: string }
interface ServerValue { name: string; value: string }
const { t } = useI18n()
const processes = ref<Process[]>([])
const status = ref<ServerValue[]>([])
const variables = ref<ServerValue[]>([])
const groups = computed(() => [{ key: 'status', values: status.value }, { key: 'variables', values: variables.value }])
const loading = ref(false)
const error = ref('')
const filter = ref('')
const autoRefresh = ref(false)
const killTarget = ref<number | null>(null)
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
function matches(v: ServerValue) { return `${v.name} ${v.value}`.toLowerCase().includes(filter.value.toLowerCase()) }
async function load() {
  if (loading.value || disposed) return
  loading.value = true
  error.value = ''
  const results = await Promise.allSettled([
    api<{ processes: Process[] }>('GET', '/api/processes').then(v => { if (!disposed) processes.value = v.processes }),
    api<{ status: ServerValue[] }>('GET', '/api/status').then(v => { if (!disposed) status.value = v.status }),
    api<{ variables: ServerValue[] }>('GET', '/api/variables').then(v => { if (!disposed) variables.value = v.variables }),
  ])
  for (const result of results) if (result.status === 'rejected') error.value = String(result.reason instanceof Error ? result.reason.message : result.reason)
  loading.value = false
}
async function kill() {
  if (killTarget.value === null || loading.value) return
  error.value = ''
  loading.value = true
  try {
    await api('POST', `/api/processes/${killTarget.value}/kill`)
    killTarget.value = null
  } catch (e) { error.value = e instanceof Error ? e.message : String(e); loading.value = false; return }
  loading.value = false
  await load()
}
watch(autoRefresh, enabled => { clearInterval(timer); timer = enabled ? setInterval(() => void load(), 5000) : undefined })
onMounted(load)
onUnmounted(() => { disposed = true; clearInterval(timer) })
</script>
<style scoped>
.admin-view{display:flex;flex-direction:column;gap:12px;max-width:100%}.toolbar{display:flex;align-items:center;gap:18px;flex-wrap:wrap}.table-scroll{max-height:400px;overflow:auto}table{width:100%;border-collapse:collapse;text-align:left}th,td{padding:8px 12px;border-bottom:1px solid #ddd;vertical-align:top}th{position:sticky;top:0;background:var(--n-color,#fff)}pre{white-space:pre-wrap;max-width:600px;margin:0;overflow-wrap:anywhere}.confirmation{padding:16px;border:1px solid #d99b20;border-radius:6px}button{cursor:pointer;padding:6px 12px}label{display:flex;align-items:center;gap:8px}[role=alert]{color:#bd2424}
</style>

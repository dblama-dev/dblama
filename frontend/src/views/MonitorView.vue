<template>
  <section class="admin-view db-page tool-workspace">
    <ToolHeading icon="activity" :title="t('admin.monitor')">
      <label class="auto-refresh">
        <input v-model="autoRefresh" data-test="auto-refresh" type="checkbox" />
        {{ t('admin.autoRefresh') }}
      </label>
      <ui-button data-test="refresh" :disabled="loading" @click="load">
        <template #icon><UiIcon name="refresh" /></template>
        {{ t('admin.refresh') }}
      </ui-button>
    </ToolHeading>
    <p v-if="error && killTarget === null" class="error" role="alert">{{ error }}</p>
    <span v-if="loading" role="status" class="tool-note">{{ t('admin.loading') }}</span>
    <UiModal :show="killTarget !== null" preset="dialog" :title="t('admin.kill')"
      :closable="!loading" :close-on-esc="!loading" :mask-closable="!loading" @update:show="killTarget = null">
      <p>{{ t('admin.confirmKill', { id: killTarget }) }}</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <template #footer>
        <ui-button data-cancel :disabled="loading" @click="killTarget = null">{{ t('admin.cancel') }}</ui-button>
        <ui-button data-test="confirm-kill" type="error" :disabled="loading" @click="kill">
          {{ t('admin.kill') }}
        </ui-button>
      </template>
    </UiModal>
    <section class="tool-card monitor-panel">
      <header class="tool-card__head">
        <h2>{{ t('admin.processes') }}</h2>
      </header>
      <div class="tool-scroll admin-records processes">
        <table class="tool-grid">
          <thead>
            <tr>
              <th>ID</th>
              <th>{{ t('admin.user') }}</th>
              <th>{{ t('admin.host') }}</th>
              <th>{{ t('admin.database') }}</th>
              <th>{{ t('admin.command') }}</th>
              <th>{{ t('admin.seconds') }}</th>
              <th>{{ t('admin.state') }}</th>
              <th>{{ t('admin.query') }}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in processes" :key="p.id">
              <td data-field="ID">{{ p.id }}</td>
              <td :data-field="t('admin.user')">{{ p.user }}</td>
              <td :data-field="t('admin.host')">{{ p.host }}</td>
              <td :data-field="t('admin.database')">{{ p.database }}</td>
              <td :data-field="t('admin.command')">{{ p.command }}</td>
              <td :data-field="t('admin.seconds')">{{ p.seconds }}</td>
              <td :data-field="t('admin.state')">{{ p.state }}</td>
              <td class="record-wide" :data-field="t('admin.query')">
                <pre>{{ p.query }}</pre>
              </td>
              <td class="record-wide">
                <ui-button
                  data-test="kill"
                  size="tiny"
                  secondary
                  type="error"
                  :disabled="loading"
                  @click="killTarget = p.id"
                >
                  {{ t('admin.kill') }}
                </ui-button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!processes.length && !loading" class="tool-empty">{{ t('admin.empty') }}</p>
    </section>
    <div class="tool-card monitor-panel">
      <div class="tool-card__head">
        <label class="metric-filter">
          <UiIcon name="search" :size="16" />
          <input
            v-model="filter"
            type="search"
            :aria-label="t('admin.filter')"
            :placeholder="t('admin.filter')"
          />
        </label>
      </div>
      <div class="metric-groups">
        <section v-for="group in groups" :key="group.key" class="metric-group">
          <h2>{{ t(`admin.${group.key}`) }}</h2>
          <div class="tool-scroll">
            <table class="tool-grid">
              <thead>
                <tr>
                  <th>{{ t('admin.name') }}</th>
                  <th>{{ t('admin.value') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="v in group.values.filter(matches)" :key="v.name">
                  <td>{{ v.name }}</td>
                  <td>{{ v.value }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import ToolHeading from '../components/ToolHeading.vue'
import UiIcon from '../components/UiIcon.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { UiButton, UiModal } from '../ui'
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
<style scoped src="./adminRecords.css"></style>
<style scoped>
.auto-refresh,
.metric-filter {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--db-muted);
}
.metric-filter input {
  width: min(320px, 67vw);
}
.monitor-panel {
  overflow: hidden;
}
.monitor-panel .tool-scroll {
  max-height: 370px;
}
.monitor-panel th {
  position: sticky;
  top: 0;
  z-index: 1;
}
.processes td {
  vertical-align: top;
}
.processes pre {
  min-width: 100px;
  max-width: 550px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  margin: 0;
  font: 12px/1.5 var(--db-mono);
}
.metric-groups {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.metric-group {
  min-width: 0;
}
.metric-group + .metric-group {
  border-inline-start: 1px solid var(--db-line);
}
.metric-group h2 {
  padding: 13px 16px;
}
.metric-group td {
  font-family: var(--db-mono);
  overflow-wrap: anywhere;
}
.error {
  color: var(--db-danger, #b95959);
  overflow-wrap: anywhere;
}
@media (max-width: 1200px) {
  .metric-groups {
    grid-template-columns: minmax(0, 1fr);
  }
  .metric-group + .metric-group {
    border-inline-start: 0;
    border-top: 1px solid var(--db-line);
  }
}
@media (max-width: 820px) {
  .monitor-panel .processes { max-height: min(65dvh, 520px); }
}
@container records (max-width: 660px) {
  .processes tbody tr { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .processes pre { min-width: 0; }
  .processes td:empty { display: none; }
  .processes td:has(pre:empty) { display: none; }
}
</style>

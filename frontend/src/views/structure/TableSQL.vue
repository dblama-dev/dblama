<template>
  <section class="ddl">
    <header><code>DDL</code>
    <ui-button size="small" data-testid="sql-refresh" :loading="loading" @click="load">
      <template #icon><UiIcon name="refresh" /></template>{{ t('table.sqlRefresh') }}
    </ui-button></header>
    <ui-alert v-if="error" type="error">{{ error }}</ui-alert>
    <p v-if="loading">{{ t('table.sqlLoading') }}</p>
    <pre v-else-if="statement !== null"><code>{{ statement }}</code></pre>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiAlert, UiButton } from '../../ui'
import UiIcon from '../../components/UiIcon.vue'
import { api } from '../../api/client'
import { useCatalogStore } from '../../stores/catalog'

const props = defineProps<{ db: string; table: string }>()
const { t } = useI18n()
const catalog = useCatalogStore()
const path = computed(() => catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}/create-sql`))
const statement = ref<string | null>(null)
const error = ref('')
const loading = ref(false)
let sequence = 0
async function load(): Promise<void> {
  const current = ++sequence
  statement.value = null
  error.value = ''
  loading.value = true
  try {
    const result = await api<{ sql: string }>('GET', path.value)
    if (current === sequence) statement.value = result.sql
  } catch (err) {
    if (current === sequence) error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (current === sequence) loading.value = false
  }
}
watch(path, () => { void load() }, { immediate: true })
onBeforeUnmount(() => { sequence++ })
</script>

<style scoped>
.ddl { min-width:0; overflow:hidden; border:1px solid var(--db-line); border-radius:10px; background:var(--db-panel); }
.ddl header { display:flex; justify-content:space-between; align-items:center; gap:8px; padding:10px 14px; border-bottom:1px solid var(--db-line); }
.ddl header code { font-family:var(--db-mono); font-weight:600; }
.ddl pre { margin:0; padding:18px; min-height:120px; background:var(--db-sidebar); color:var(--db-ink); white-space:pre-wrap; overflow-wrap:anywhere; }
.ddl pre code { font:13px/1.7 var(--db-mono); }
.ddl p { padding:14px; color:var(--db-muted); }
</style>

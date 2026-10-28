<template>
  <section>
    <n-button size="small" data-testid="sql-refresh" :loading="loading" @click="load">
      {{ t('table.sqlRefresh') }}
    </n-button>
    <n-alert v-if="error" type="error">{{ error }}</n-alert>
    <p v-if="loading">{{ t('table.sqlLoading') }}</p>
    <pre v-else-if="statement !== null"><code>{{ statement }}</code></pre>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton } from 'naive-ui'
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
pre { overflow: auto; padding: 16px; background: rgba(128, 128, 128, 0.08); white-space: pre-wrap; overflow-wrap: anywhere; }
</style>

<template>
  <section v-if="enabled" class="table-transfer">
    <n-space>
      <n-button :disabled="!tables.length || busy" @click="open('copy')">{{ t('databaseOps.copyTables') }}</n-button>
      <n-button :disabled="!tables.length || busy" @click="open('move')">{{ t('databaseOps.moveTables') }}</n-button>
      <n-button v-if="completedTarget" @click="openTarget">{{ t('databaseOps.openTarget') }}</n-button>
    </n-space>
    <form v-if="action" class="transfer-form" @submit.prevent="submit">
      <span>{{ t('databaseOps.selected', { count: tables.length }) }}</span>
      <n-alert type="info">{{ t('databaseOps.transferWarning') }}</n-alert>
      <n-alert v-if="action === 'move'" type="warning">{{ t('databaseOps.moveWarning') }}</n-alert>
      <label :for="targetID">{{ t('databaseOps.target') }}</label>
      <n-select :input-props="{ id: targetID }" v-model:value="target" :options="targets" :loading="loading" :disabled="busy" filterable />
      <n-alert v-if="error" type="error" role="alert">{{ error }}</n-alert>
      <n-space>
        <n-button attr-type="submit" type="primary" :disabled="!target || !tables.length || loading || busy" :loading="busy">{{ t(`databaseOps.${action}`) }}</n-button>
        <n-button :disabled="busy" @click="action = null">{{ t('databaseOps.cancel') }}</n-button>
      </n-space>
    </form>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NSelect, NSpace } from 'naive-ui'
import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import { transferTables } from '../lib/databaseOps'
import type { DatabaseInfo } from '../api/types'
const props = defineProps<{ db: string; tables: string[] }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const router = useRouter()
const session = useSessionStore()
const enabled = computed(() => session.sessionInfo?.caps.moveCopy === true)
const action = ref<'copy' | 'move' | null>(null)
const target = ref<string | null>(null)
const targets = ref<{ label: string; value: string }[]>([])
const completedTarget = ref('')
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const targetID = `transfer-target-${useId()}`
watch(() => props.db, () => { action.value = null; target.value = null; completedTarget.value = '' })
async function open(mode: 'copy' | 'move'): Promise<void> {
  action.value = mode; error.value = ''; target.value = null; loading.value = true
  try {
    const payload = await api<{ databases: DatabaseInfo[] }>('GET', '/api/databases')
    targets.value = payload.databases.filter(d => d.name !== props.db).map(d => ({ label: d.name, value: d.name }))
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { loading.value = false }
}
async function submit(): Promise<void> {
  if (!enabled.value || !target.value || !action.value || busy.value || !props.tables.length) return
  busy.value = true; error.value = ''
  try {
    await transferTables(props.db, [...props.tables], target.value, action.value)
    completedTarget.value = target.value; action.value = null; emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
function openTarget(): void { void router.push({ name: 'db', params: { db: completedTarget.value } }) }
</script>
<style scoped>
.table-transfer, .transfer-form { display: flex; flex-direction: column; gap: 12px; }
.transfer-form { max-width: 640px; padding: 16px; border: 1px solid var(--n-border-color, #8885); border-radius: 6px; }
</style>

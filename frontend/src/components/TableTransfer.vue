<template>
  <section v-if="enabled && (tables.length || completedTarget)" class="table-transfer">
    <div class="db-toolbar">
      <ui-button v-if="tables.length" size="small" :disabled="busy" :aria-expanded="action === 'copy'" @click="open('copy')">{{ t('databaseOps.copyTables') }}</ui-button>
      <ui-button v-if="tables.length" size="small" :disabled="busy" :aria-expanded="action === 'move'" @click="open('move')">{{ t('databaseOps.moveTables') }}</ui-button>
      <ui-button size="small" v-if="completedTarget" @click="openTarget">{{ t('databaseOps.openTarget') }}</ui-button>
    </div>
    <form v-if="action && tables.length" class="transfer-form" @submit.prevent="submit">
      <strong>{{ t(action === 'copy' ? 'databaseOps.copyTables' : 'databaseOps.moveTables') }}</strong>
      <ui-alert type="info">{{ t('databaseOps.transferWarning') }}</ui-alert>
      <ui-alert v-if="action === 'move'" type="warning">{{ t('databaseOps.moveWarning') }}</ui-alert>
      <label :for="targetID">{{ t('databaseOps.target') }}</label>
      <ui-select :input-props="{ id: targetID }" v-model:value="target" :options="targets" :loading="loading" :disabled="busy" filterable />
      <ui-alert v-if="error" type="error" role="alert">{{ error }}</ui-alert>
      <div class="db-toolbar">
        <ui-button attr-type="submit" type="primary" :disabled="!target || !tables.length || loading || busy" :loading="busy">{{ t(`databaseOps.${action}`) }}</ui-button>
        <ui-button :disabled="busy" @click="action = null">{{ t('databaseOps.cancel') }}</ui-button>
      </div>
    </form>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { UiAlert, UiButton, UiSelect } from '../ui'
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
watch(() => props.tables.length, count => {
  if (!count) { action.value = null; target.value = null }
})
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
  const source = props.db
  const destination = target.value
  try {
    await transferTables(source, [...props.tables], destination, action.value)
    if (props.db === source) {
      completedTarget.value = destination
      action.value = null
      emit('changed')
    }
  } catch (e) { if (props.db === source) error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
function openTarget(): void { void router.push({ name: 'db', params: { db: completedTarget.value } }) }
</script>
<style scoped>
.table-transfer, .transfer-form { display: flex; flex-direction: column; gap: 12px; }
.transfer-form { max-width: 640px; padding: 16px; border: 1px solid var(--db-line, #8885); border-radius: 6px; }
</style>

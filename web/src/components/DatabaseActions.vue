<template>
  <section class="database-actions">
    <div class="database-action-head"><slot name="heading" /><div class="db-toolbar">
      <ui-button v-if="!db" type="primary" :disabled="busy" @click="open('create')"><UiIcon name="plus" />{{ t('databaseOps.createDatabase') }}</ui-button>
      <ui-button v-if="db" :disabled="busy" @click="open('rename')">{{ t('databaseOps.renameDatabase') }}</ui-button>
      <ui-button v-if="db" type="error" secondary :disabled="busy" @click="open('drop')">{{ t('databaseOps.dropDatabase') }}</ui-button>
    </div></div>
    <form v-if="mode" class="database-form" @submit.prevent="submit">
      <ui-alert v-if="mode === 'drop'" type="warning">{{ t('databaseOps.dropWarning') }}</ui-alert>
      <ui-alert v-if="mode === 'rename' && !postgres" type="warning">{{ t('databaseOps.renameWarning') }}</ui-alert>
      <ui-alert v-if="mode !== 'create' && db === session.sessionInfo?.database" type="info">{{ t('databaseOps.connectedWarning') }}</ui-alert>
      <label :for="fieldID">{{ mode === 'drop' ? t('databaseOps.confirmName', { name: db }) : t('databaseOps.name') }}</label>
      <ui-input :input-props="{ id: fieldID }" v-model:value="name" :disabled="busy" autocomplete="off" />
      <template v-if="mode === 'create'">
        <label :for="`${fieldID}-collation`">{{ t('databaseOps.collation') }}</label>
        <ui-select :input-props="{ id: `${fieldID}-collation` }" v-model:value="collation" :options="collations" clearable filterable :loading="loadingMetadata" :disabled="busy" :placeholder="t('databaseOps.serverDefault')" />
      </template>
      <ui-alert v-if="error" type="error" role="alert">{{ error }}</ui-alert>
      <div class="db-toolbar">
        <ui-button attr-type="submit" :type="mode === 'drop' ? 'error' : 'primary'" :loading="busy" :disabled="!valid || busy">{{ t(`databaseOps.${mode === 'drop' ? 'confirmDrop' : mode}`) }}</ui-button>
        <ui-button :disabled="busy" @click="mode = null">{{ t('databaseOps.cancel') }}</ui-button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiAlert, UiButton, UiInput, UiSelect } from '../ui'
import UiIcon from './UiIcon.vue'
import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
import { createDatabase, dropDatabase, renameDatabase, validDatabaseName, type DatabaseMetadata } from '../lib/databaseOps'

const props = defineProps<{ db?: string }>()
const emit = defineEmits<{ changed: []; renamed: [name: string]; dropped: [name: string] }>()
const { t } = useI18n()
const session = useSessionStore()
const catalog = useCatalogStore()
const postgres = computed(() => session.sessionInfo?.driver?.flavor === 'postgres')
const fieldID = `database-name-${useId()}`
const mode = ref<'create' | 'rename' | 'drop' | null>(null)
const name = ref('')
const collation = ref<string | null>(null)
const collations = ref<{ label: string; value: string }[]>([])
const loadingMetadata = ref(false)
const busy = ref(false)
const error = ref('')
const valid = computed(() => mode.value === 'drop' ? name.value === props.db : validDatabaseName(name.value) && (mode.value !== 'rename' || name.value !== props.db))
watch(() => props.db, () => { mode.value = null; error.value = '' })
async function open(action: 'create' | 'rename' | 'drop'): Promise<void> {
  mode.value = action; name.value = action === 'rename' ? (props.db ?? '') : ''; error.value = ''
  if (action !== 'create') return
  loadingMetadata.value = true
  try {
    const metadata = await api<DatabaseMetadata>('GET', '/api/metadata')
    collations.value = metadata.collations.filter(c => c.database).map(c => ({ label: c.name, value: c.name }))
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { loadingMetadata.value = false }
}
async function submit(): Promise<void> {
  if (!valid.value || busy.value || !mode.value) return
  busy.value = true; error.value = ''
  try {
    if (mode.value === 'create') await createDatabase(name.value, collation.value ?? '')
    else if (mode.value === 'rename' && props.db) { await renameDatabase(props.db, name.value); emit('renamed', name.value) }
    else if (mode.value === 'drop' && props.db) { await dropDatabase(props.db); emit('dropped', props.db) }
    catalog.databaseRevision++
    mode.value = null; emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
</script>
<style scoped>
.database-actions, .database-form { display: flex; flex-direction: column; gap: 12px; }
.database-action-head { display: flex; justify-content: space-between; flex-wrap: wrap; align-items: center; gap: 10px; }
.database-form { background: var(--db-sidebar); max-width: 640px; padding: 16px; border: 1px solid var(--db-line); border-radius: 9px; }
</style>

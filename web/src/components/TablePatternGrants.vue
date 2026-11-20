<template>
  <section class="table-pattern-grants" :aria-label="t('pattern')">
    <div class="pattern-row">
      <label>{{ t('pattern') }}
        <input v-model="pattern" data-test="table-pattern" autocomplete="off" spellcheck="false"
          aria-describedby="table-mask-help" :aria-invalid="!!pattern && !matcher" :disabled="locked">
      </label>
      <n-button v-if="username" data-test="table-prefix" :disabled="locked"
        @click="pattern = usernamePrefixPattern(username)">{{ t('prefix') }}</n-button>
    </div>
    <p id="table-mask-help" class="hint">{{ t('syntax') }}</p>
    <p v-if="pattern && !matcher" role="alert">{{ t('invalid') }}</p>
    <p v-if="loading" role="status">{{ t('loading') }}</p>
    <div v-else-if="loadError" class="pattern-row">
      <p role="alert">{{ loadError }}</p>
      <n-button data-test="retry-table-list" @click="loadTables">{{ t('retryList') }}</n-button>
    </div>
    <div v-else class="preview">
      <strong role="status">{{ t('targets', { database, count: matches.length }) }}</strong>
      <ul v-if="matches.length" data-test="table-matches" tabindex="0">
        <li v-for="name in matches" :key="name"><code>{{ name }}</code></li>
      </ul>
      <p class="hint">{{ t('effect') }}</p>
    </div>
    <fieldset>
      <legend>{{ t('privileges') }}</legend>
      <div v-if="available.length" class="privilege-list">
        <label v-for="name in available" :key="name">
          <input v-model="selected" type="checkbox" :value="name" :data-test="'table-privilege-' + name" :disabled="locked">
          <code>{{ name }}</code>
        </label>
      </div>
      <p v-else>{{ t('noPrivileges') }}</p>
    </fieldset>
    <div v-if="busy" role="status">{{ t('progress', { done: processed, total: attemptSize }) }}</div>
    <div v-if="batch && !busy" data-test="table-grant-result" class="result">
      <strong role="status">{{ t('result', { done: batch.total - batch.failures.length, total: batch.total }) }}</strong>
      <div v-if="batch.failures.length" role="alert">
        <span>{{ t('failed') }}</span>
        <ul><li v-for="failure in batch.failures" :key="failure.table"><code>{{ failure.table }}</code> · {{ failure.message }}</li></ul>
      </div>
      <div class="actions">
        <n-button v-if="batch.failures.length" data-test="retry-table-grants" type="primary" :disabled="disabled" @click="grant">{{ t('retry') }}</n-button>
        <n-button :disabled="disabled" @click="startAgain">{{ t('again') }}</n-button>
      </div>
    </div>
    <div v-else class="actions">
      <n-button type="primary" data-test="apply-table-grants" :disabled="!canGrant" @click="grant">{{ t('apply') }}</n-button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, useThemeVars } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import type { TableInfo } from '../api/types'
import messages from '../i18n/locales/tablePatternGrants.json'
import { compileGrantPattern, usernamePrefixPattern } from '../lib/grantPattern'

const props = defineProps<{
  path: string
  database: string
  username: string
  catalog: { name: string; levels: string[] }[]
  disabled?: boolean
}>()
const emit = defineEmits<{ busy: [boolean]; dirty: [boolean]; changed: [] }>()
const { t } = useI18n({ useScope: 'local', messages })
const theme = useThemeVars()
const pattern = ref('')
const selected = ref<string[]>([])
const names = ref<string[]>([])
const loading = ref(false)
const loadError = ref('')
const busy = ref(false)
const processed = ref(0)
const attemptSize = ref(0)

interface Failure { table: string; message: string }
interface Batch { path: string; database: string; privileges: string[]; total: number; failures: Failure[] }
const batch = ref<Batch | null>(null)
const locked = computed(() => !!props.disabled || busy.value || batch.value !== null)
const available = computed(() => props.catalog.filter(p => p.levels.includes('table')).map(p => p.name))
const matcher = computed(() => compileGrantPattern(pattern.value))
const matches = computed(() => matcher.value ? names.value.filter(matcher.value) : [])
const privileges = computed(() => selected.value.filter(name => available.value.includes(name)))
const canGrant = computed(() => !locked.value && !loading.value && !loadError.value && matches.value.length > 0 && privileges.value.length > 0)
const dirty = computed(() => batch.value ? batch.value.failures.length > 0 : !!(pattern.value || selected.value.length))
watch(dirty, value => emit('dirty', value), { flush: 'sync' })
watch(busy, value => emit('busy', value), { flush: 'sync' })
let disposed = false
onBeforeUnmount(() => {
  disposed = true
  if (busy.value) emit('busy', false)
  emit('dirty', false)
})

async function loadTables() {
  loading.value = true
  names.value = []
  loadError.value = ''
  try {
    const result = await api<{ tables: TableInfo[] }>('GET', '/api/databases/' + encodeURIComponent(props.database) + '/tables')
    names.value = [...new Set(result.tables.filter(table => !table.isView).map(table => table.name))]
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : String(error)
  } finally { loading.value = false }
}
onMounted(loadTables)

async function grant() {
  if (props.disabled || busy.value) return
  const previous = batch.value
  if (previous ? previous.failures.length === 0 : !canGrant.value) return
  const targets = previous ? previous.failures.map(f => f.table) : [...matches.value]
  const snapshot = previous ?? {
    path: props.path, database: props.database, privileges: [...privileges.value], total: targets.length,
  }
  busy.value = true
  processed.value = 0
  attemptSize.value = targets.length
  const failures: Failure[] = []
  try {
    for (const table of targets) {
      try {
        await api('POST', snapshot.path, {
          scope: { level: 'table', database: snapshot.database, table },
          privileges: snapshot.privileges, grantOption: false,
        })
      } catch (error) {
        failures.push({ table, message: error instanceof Error ? error.message : String(error) })
      }
      if (disposed) return
      processed.value++
    }
    batch.value = { ...snapshot, failures }
  } finally { busy.value = false }
  // A transport error can hide a successful write. Read back every completed batch.
  emit('changed')
}

function startAgain() {
  pattern.value = ''
  selected.value = []
  batch.value = null
  void loadTables()
}
</script>

<style scoped>
.table-pattern-grants, .preview, .result { display: grid; gap: 8px; min-width: 0; }
.table-pattern-grants { gap: 12px; }
.pattern-row, .actions { display: flex; flex-wrap: wrap; align-items: end; gap: 8px; }
.pattern-row label { display: grid; gap: 4px; min-width: 0; }
.pattern-row input { min-width: 0; font: inherit; padding: 6px 10px; color: v-bind('theme.textColor1'); background: v-bind('theme.inputColor'); border: 1px solid v-bind('theme.borderColor'); border-radius: 4px; }
p, ul { margin: 0; }
.hint { color: v-bind('theme.textColor2'); }
.preview, fieldset { min-width: 0; padding: 8px 12px; border: 1px solid v-bind('theme.borderColor'); border-radius: 5px; }
fieldset { margin: 0; }
legend { font-weight: 600; padding-inline: 4px; }
ul { padding-inline-start: 1.5em; max-height: 12em; overflow: auto; }
li, strong, code { overflow-wrap: anywhere; }
.privilege-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(170px, 100%), 1fr)); gap: 5px 12px; }
.privilege-list label { display: flex; align-items: center; gap: 6px; }
.privilege-list input { accent-color: v-bind('theme.primaryColor'); width: 16px; height: 16px; flex-shrink: 0; margin: 0; }
:where(input, ul):focus-visible { outline: 2px solid v-bind('theme.primaryColor'); outline-offset: 2px; }
[role=alert] { color: v-bind('theme.errorColor'); }
</style>

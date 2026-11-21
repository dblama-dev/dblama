<template>
  <button ref="trigger" class="jump-trigger" type="button" :title="t('shell.jump') + ' (' + shortcut + ')'" :aria-label="t('shell.jump')" @click="open">
    <UiIcon name="search" :size="15" /><span>{{ t('navigationWorkflows.search') }}</span><kbd>{{ shortcut }}</kbd>
  </button>
  <ui-modal v-model:show="visible" :title="t('shell.jump')" :auto-focus="false"
    class="jump-card" style="width: min(490px, calc(100vw - 24px))" @after-enter="searchInput?.focus()">
    <div class="jump" @keydown.down.prevent="step(1)" @keydown.up.prevent="step(-1)">
      <label class="jump-input">
        <UiIcon name="search" :size="17" />
        <input ref="searchInput" v-model="search" type="search" :aria-label="t('shell.jump')" :placeholder="t('shell.jumpPlaceholder')"
          @keydown.enter.prevent="go(results[0])" />
      </label>
      <div class="jump-results">
        <button v-for="(item, index) in results" :key="item.path" class="jump-result" type="button" :data-result="index" @click="go(item)">
          <UiIcon :name="item.icon" :size="16" /><span>{{ item.label }}</span><small>{{ item.group }}</small>
        </button>
      </div>
      <p v-if="error" class="jump-error" role="alert">{{ error }}</p>
      <p v-if="loading || (!results.length && !error)" class="jump-empty" role="status">{{ t(loading ? 'shell.loading' : 'shell.noResults') }}</p>
    </div>
  </ui-modal>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { UiModal } from '../ui'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import type { DatabaseInfo, TableInfo } from '../api/types'
import { databaseTools, objectDestinations, serverDestinations } from '../lib/workspaceNavigation'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import UiIcon from './UiIcon.vue'

interface Match { label: string; icon: string; path: string; group: string }
const { t } = useI18n()
const route = useRoute(), router = useRouter(), catalog = useCatalogStore(), session = useSessionStore()
const searchInput = ref<HTMLInputElement>()
const trigger = ref<HTMLButtonElement>()
const visible = ref(false), search = ref(''), loading = ref(false), error = ref('')
const databases = ref<DatabaseInfo[]>([]), tables = ref<TableInfo[]>([])
const shortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ K' : 'Ctrl K'
const db = computed(() => typeof route.params.db === 'string' ? route.params.db : null)
const base = computed(() => db.value ? '/db/' + encodeURIComponent(db.value) : '')
let requestId = 0

const results = computed<Match[]>(() => {
  const caps = session.sessionInfo?.caps
  const needle = search.value.trim().toLocaleLowerCase()
  const options: Match[] = []
  const add = (label: string, icon: string, path: string, group: string) => {
    if (label.toLocaleLowerCase().includes(needle)) options.push({label, icon, path, group})
  }
  if (db.value) {
    const group = db.value
    add(t('sideNavigation.overview'), 'overview', catalog.path(base.value), group)
    for (const [action, icon] of Object.entries(databaseTools)) {
      add(t('navigationWorkflows.' + action), icon, catalog.path(base.value + '/' + action), group)
    }
    for (const item of objectDestinations(group, caps)) add(t(item.label), item.icon, catalog.path(item.path), group)
  }
  const server = t('sideNavigation.server')
  add(t('server.title'), 'server', '/server', server)
  for (const item of serverDestinations(caps)) add(t(item.label), item.icon, item.path, server)
  if (db.value) {
    const tableLabel = t('tree.tables'), viewLabel = t('tree.views')
    for (const table of tables.value) {
      add(table.name, table.isView ? 'eye' : 'table', catalog.path(base.value + '/table/' + encodeURIComponent(table.name)), table.isView ? viewLabel : tableLabel)
      if (options.length >= 40) break
    }
  } else {
    for (const database of databases.value) {
      add(database.name, 'db', '/db/' + encodeURIComponent(database.name), t('sideNavigation.database'))
      if (options.length >= 40) break
    }
  }
  return options
})

async function open(): Promise<void> {
  if (visible.value) return
  visible.value = true
  search.value = ''
  error.value = ''
  databases.value = []
  tables.value = []
  const current = ++requestId
  await nextTick()
  if (!visible.value || current !== requestId) return
  searchInput.value?.focus()
  const path = db.value ? catalog.path('/api/databases/' + encodeURIComponent(db.value) + '/tables') : '/api/databases'
  if (db.value && catalog.tablesPath === path && !catalog.error && !catalog.loading) {
    tables.value = catalog.tables
    return
  }
  loading.value = true
  try {
    if (db.value) {
      const payload = await api<{tables: TableInfo[]}>('GET', path)
      if (current === requestId) tables.value = payload.tables ?? []
    } else {
      const payload = await api<{databases: DatabaseInfo[]}>('GET', path)
      if (current === requestId) databases.value = payload.databases ?? []
    }
  } catch (err) {
    if (current === requestId) error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (current === requestId) loading.value = false
  }
}
function step(delta: number) {
  const input = searchInput.value
  if (!input) return
  const buttons = Array.from(input.closest('.jump')?.querySelectorAll<HTMLButtonElement>('.jump-result') ?? [])
  if (!buttons.length) return
  const current = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const next = current < 0 ? (delta > 0 ? 0 : buttons.length - 1) : (current + delta + buttons.length) % buttons.length
  buttons[next]?.focus()
}
function go(item: Match | undefined) {
  if (!item) return
  visible.value = false
  void router.push(item.path)
}
function shortcutPressed(event: KeyboardEvent) {
  if (event.key.toLowerCase() !== 'k' || !(event.ctrlKey || event.metaKey) || event.altKey) return
  if (visible.value) { event.preventDefault(); searchInput.value?.focus(); return }
  // A command should never take focus away from a row editor, confirmation or mobile drawer.
  if (!trigger.value || trigger.value.closest('[inert]') || document.querySelector('.ui-modal, [role="alertdialog"], dialog[open]')) return
  event.preventDefault()
  void open()
}
watch(visible, value => {
  if (!value) { requestId++; loading.value = false }
})
watch(() => route.fullPath, () => { visible.value = false })
onMounted(() => window.addEventListener('keydown', shortcutPressed))
onBeforeUnmount(() => { window.removeEventListener('keydown', shortcutPressed); requestId++ })
</script>

<style scoped>
.jump-trigger { display: inline-flex; align-items: center; gap: 7px; min-height: 29px; padding: 0 7px; border: 1px solid var(--db-line); border-radius: 6px; color: var(--db-muted); background: var(--db-panel); cursor: pointer; font-size: 12px; }
.jump-trigger:hover { color: var(--db-accent); border-color: var(--db-accent); }
.jump-trigger kbd { font: 10px var(--db-mono); padding: 2px 3px; border: 1px solid var(--db-line); border-radius: 3px; }
.jump { display: grid; gap: 10px; }
.jump-input { display: flex; align-items: center; gap: 9px; border: 1px solid var(--db-line); border-radius: 7px; padding: 0 10px; color: var(--db-muted); }
.jump-input:focus-within { outline: 2px solid var(--db-accent); outline-offset: 1px; }
.jump-input input { width: 100%; min-height: 37px; border: none; outline: none; background: transparent; color: var(--db-ink); font: inherit; }
.jump-results { max-height: min(54vh, 410px); overflow: auto; }
.jump-result { width: 100%; display: flex; align-items: center; gap: 9px; box-sizing: border-box; padding: 8px; text-align: start; border: 0; border-radius: 6px; background: none; color: var(--db-ink); font: inherit; cursor: pointer; }
.jump-result:hover, .jump-result:focus-visible { background: var(--db-tint); color: var(--db-accent); }
.jump-result span { min-width: 0; overflow-wrap: anywhere; }
.jump-result small { margin-left: auto; overflow: hidden; text-overflow: ellipsis; max-width: 32%; color: var(--db-muted); }
.jump-result svg { color: var(--db-muted); }
.jump-empty, .jump-error { margin: 6px 0; color: var(--db-muted); }
.jump-error { color: var(--db-danger, #c44854); }
@media (max-width: 1030px) { .jump-trigger kbd { display: none; } }
@media (max-width: 719px) { .jump-trigger { min-width: 29px; padding: 0; justify-content: center; border: none; } .jump-trigger span { display: none; } }
</style>

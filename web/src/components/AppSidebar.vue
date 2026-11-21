<template>
  <aside class="app-sidebar" :aria-label="t('shell.navigation')">
    <router-link v-if="driver" to="/server" class="connection" :title="t('server.title')" :aria-label="t('server.title') + ' · ' + endpoint">
      <span class="connection__icon"><UiIcon name="server" :size="17" /></span>
      <span class="connection__copy">
        <strong>{{ endpoint }}</strong>
        <small>{{ driver.name }} {{ driver.version }}</small>
      </span>
      <UiIcon name="chevron-right" :size="13" />
    </router-link>
    <ObjectTree>
      <template #tools="{ db }"><AdvancedNavigation :db="db" /></template>
    </ObjectTree>
    <div v-if="driver" class="identity">
      <span class="identity__avatar" aria-hidden="true">{{ driver.user.slice(0, 2).toUpperCase() || '·' }}</span>
      <span class="identity__copy"><strong>{{ driver.user }}</strong><small>{{ t('shell.databaseUser') }}</small></span>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import AdvancedNavigation from './AdvancedNavigation.vue'
import ObjectTree from './ObjectTree.vue'
import UiIcon from './UiIcon.vue'

const { t } = useI18n()
const session = useSessionStore()
const driver = computed(() => session.sessionInfo?.driver)
const endpoint = computed(() => {
  const info = driver.value
  if (!info) return ''
  if (!info.port) return info.host
  const host = info.host.includes(':') && !info.host.startsWith('[') ? '[' + info.host + ']' : info.host
  return host + ':' + info.port
})
</script>

<style scoped>
.app-sidebar { display: flex; flex: 1; min-height: 0; flex-direction: column; color: var(--db-ink); background: var(--db-sidebar); }
.connection, .identity { display: flex; align-items: center; gap: 9px; }
.connection { margin: 14px 12px 4px; padding: 10px; border: 1px solid var(--db-line); border-radius: 9px; color: var(--db-muted); text-decoration: none; background: var(--db-panel); }
.connection:hover { border-color: var(--db-accent); }
.connection__icon { display: grid; place-items: center; width: 31px; height: 31px; flex-shrink: 0; border-radius: 7px; background: var(--db-tint); color: var(--db-accent); }
.connection__copy, .identity__copy { display: grid; min-width: 0; flex: 1; }
.connection strong, .identity strong, .connection small, .identity small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.connection strong, .identity strong { color: var(--db-ink); font: 600 12px/1.5 var(--db-mono); }
.connection small, .identity small { color: var(--db-muted); font-size: 11px; line-height: 1.4; }
.identity { flex-shrink: 0; padding: 11px 16px; border-top: 1px solid var(--db-line); }
.identity__avatar { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; border: 1px solid var(--db-line); background: var(--db-tint); color: var(--db-accent); font: 600 11px var(--db-mono); }
</style>

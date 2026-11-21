<template>
  <div class="app-topbar">
    <nav class="breadcrumbs" :aria-label="t('shell.path')">
      <router-link to="/server" class="breadcrumbs__home" :title="t('server.title')">
        <UiIcon name="server" :size="15" /><span>{{ t('sideNavigation.server') }}</span>
      </router-link>
      <template v-for="(crumb, index) in crumbs" :key="index">
        <span class="breadcrumbs__slash" aria-hidden="true">/</span>
        <router-link v-if="crumb.to" :to="crumb.to" :class="{ identifier: crumb.identifier }" :title="crumb.label">{{ crumb.label }}</router-link>
        <span v-else :class="{ identifier: crumb.identifier, 'breadcrumb-current': index === currentCrumb }" :title="crumb.label" :aria-current="index === currentCrumb ? 'page' : undefined">{{ crumb.label }}</span>
      </template>
    </nav>
    <div class="app-topbar__actions">
      <WorkspaceSearch />
      <span v-if="driver" class="session-status" :title="driver.user + ' @ ' + driver.host"><i aria-hidden="true"></i>{{ t('shell.connected') }}</span>
      <button type="button" class="icon-button" :title="t('topbar.theme')" :aria-label="t('topbar.theme')" @click="toggleTheme">
        <UiIcon name="moon" :size="16" />
      </button>
      <div class="languages" role="group" :aria-label="t('shell.language')">
        <button v-for="lang in languages" :key="lang" type="button" :aria-pressed="locale === lang" @click="locale = lang">{{ lang.toUpperCase() }}</button>
      </div>
      <button type="button" class="sign-out" :aria-label="t('topbar.logout')" :title="t('topbar.logout')" @click="logout">
        <UiIcon name="logout" :size="15" /><span>{{ t('topbar.logout') }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import { themeToggleKey } from '../theme'
import UiIcon from './UiIcon.vue'
import { objectIcons } from '../lib/workspaceNavigation'
import WorkspaceSearch from './WorkspaceSearch.vue'

const { t, locale } = useI18n()
const router = useRouter()
const route = useRoute()
const store = useSessionStore()
const catalog = useCatalogStore()
const toggleTheme = inject(themeToggleKey, () => {})
const driver = computed(() => store.sessionInfo?.driver)
const languages = ['ru', 'en'] as const
interface Crumb { label: string; to?: string; identifier?: boolean }
const routeLabels: Record<string, string> = {
  server: 'server.title', 'server-users': 'admin.users', 'server-processes': 'admin.monitor',
  'db-console': 'navigationWorkflows.console', 'db-search': 'navigationWorkflows.search',
  'db-export': 'navigationWorkflows.export', 'db-import': 'navigationWorkflows.import',
  'db-operations': 'databaseOps.databaseActions',
  'db-relationships': 'relationships.title', 'db-partitions': 'navigationWorkflows.partitions',
}
const crumbs = computed<Crumb[]>(() => {
  const result: Crumb[] = []
  const {db, t: table, kind} = route.params
  const name = String(route.name)
  if (typeof db === 'string') {
    const base = '/db/' + encodeURIComponent(db)
    result.push({label: db, identifier: true, to: name === 'db' ? undefined : catalog.path(base)})
    if (catalog.schema) result.push({label: catalog.schema, identifier: true})
    if (typeof table === 'string') {
      const newTable = table === '_new'
      result.push({
        label: newTable ? t('table.new') : table,
        identifier: !newTable,
        to: name === 'db-partitions' ? catalog.path(base + '/table/' + encodeURIComponent(table)) : undefined,
      })
    }
  }
  if (name === 'db-objects' && typeof kind === 'string') {
    const known = Object.hasOwn(objectIcons, kind)
    result.push({label: known ? t('objects.' + kind) : kind})
  } else if (routeLabels[name]) {
    result.push({label: t(routeLabels[name])})
  }
  return result
})

const currentCrumb = computed(() => route.name === 'db' ? 0 : crumbs.value.length - 1)

async function logout(): Promise<void> {
  try {
    await store.logout()
  } catch {
    // The store clears the local session even if its server request fails.
  }
  await router.push('/login')
}
</script>

<style scoped>
.app-topbar { min-width: 0; width: 100%; height: 59px; display: flex; align-items: center; justify-content: space-between; gap: 15px; color: var(--db-ink); }
.breadcrumbs, .app-topbar__actions, .breadcrumbs__home, .sign-out, .session-status, .languages, .icon-button { display: flex; align-items: center; }
.breadcrumbs { flex: 1; min-width: 0; gap: 9px; flex-wrap: wrap; line-height: 1.35; }
.breadcrumbs > span, .breadcrumbs > a { color: var(--db-muted); font-size: 12px; overflow: hidden; max-width: 28ch; text-overflow: ellipsis; white-space: nowrap; text-decoration: none; }
.breadcrumbs__home { gap: 7px; }
.breadcrumbs .breadcrumbs__slash { color: var(--db-muted); opacity: .5; }
.breadcrumbs .identifier { font-family: var(--db-mono); }
.breadcrumbs > .breadcrumb-current { color: var(--db-ink); }
.breadcrumbs a:hover { color: var(--db-accent); }
.app-topbar__actions { gap: 8px; flex-shrink: 0; }
.app-topbar button:not(.jump-trigger) { cursor: pointer; color: var(--db-muted); border: 0; background: transparent; font: inherit; }
.icon-button, .sign-out { justify-content: center; height: 30px; padding: 0 7px; border-radius: 6px; }
.sign-out { gap: 6px; }
.sign-out span { font-size: 12px; }
.app-topbar button:hover { color: var(--db-accent); background: var(--db-tint); }
.session-status { gap: 6px; margin-right: 6px; color: var(--db-muted); font-size: 11px; }
.session-status i { display: block; width: 6px; height: 6px; border-radius: 50%; background: #31a87c; }
.languages { padding: 2px; border-radius: 6px; border: 1px solid var(--db-line); }
.languages button { border-radius: 4px; font-size: 11px; line-height: 20px; padding: 0 5px; }
.languages button[aria-pressed="true"] { background: var(--db-tint); color: var(--db-accent); font-weight: 600; }
@media (max-width: 1030px) { .session-status { display:none; } .breadcrumbs > span, .breadcrumbs > a { max-width: 15ch; } }
@media (max-width: 719px) {
  .app-topbar { min-height: 48px; height: auto; gap: 5px; }
  .app-topbar__actions { gap: 2px; }
  .breadcrumbs { gap: 5px; }
  .breadcrumbs > :not(.breadcrumb-current) { display: none; }
  .breadcrumbs > .breadcrumb-current { max-width: 100%; }
  .sign-out span { display: none; }
}
</style>

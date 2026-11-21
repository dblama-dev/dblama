<template>
  <details class="advanced-navigation" :open="advancedRoute">
    <summary>
      <UiIcon name="more" :size="16" /><span>{{ t('sideNavigation.more') }}</span><UiIcon class="disclosure" name="chevron-right" :size="13" />
    </summary>
    <nav :aria-label="t('sideNavigation.more')">
      <router-link v-for="item in tools" :key="item.path" :to="catalog.path(item.path)">
        <UiIcon :name="item.icon" :size="15" />{{ t(item.label) }}
      </router-link>
    </nav>
  </details>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
import UiIcon from './UiIcon.vue'
import { databaseDestinations } from '../lib/workspaceNavigation'
const { t } = useI18n()
const props = defineProps<{ db: string }>()
const session = useSessionStore()
const catalog = useCatalogStore()
const route = useRoute()
const tools = computed(() => databaseDestinations(props.db, session.sessionInfo?.caps))
const advancedRoute = computed(() => ['db-operations', 'db-relationships', 'db-objects'].includes(String(route.name)))
</script>
<style scoped>
summary, a { display: flex; align-items: center; gap: 9px; padding: 6px 9px; min-height: 30px; box-sizing: border-box; border-radius: 6px; line-height: 1.3; font-size: 13px; color: var(--db-ink); }
summary { list-style: none; cursor: pointer; }
summary::-webkit-details-marker { display: none; }
summary svg, a svg { color: var(--db-muted); }
.disclosure { margin-left: auto; }
details[open] .disclosure { transform: rotate(90deg); }
nav { margin: 4px 0 4px 15px; padding-left: 6px; border-left: 1px solid var(--db-line); }
a { text-decoration: none; overflow-wrap: anywhere; }
summary:hover, a:hover, .router-link-exact-active { background: var(--db-tint); color: var(--db-accent); }
.router-link-exact-active svg { color: var(--db-accent); }
</style>

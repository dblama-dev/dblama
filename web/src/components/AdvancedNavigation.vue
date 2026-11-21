<template>
  <details v-if="objects.length || server.length" class="advanced-navigation" :open="advancedRoute">
    <summary>
      <UiIcon name="more" :size="16" /><span>{{ t('sideNavigation.more') }}</span><UiIcon class="disclosure" name="chevron-right" :size="13" />
    </summary>
    <nav :aria-label="t('sideNavigation.more')">
      <section v-if="objects.length">
        <strong>{{ t('sideNavigation.databaseObjects') }}</strong>
        <router-link v-for="item in objects" :key="item.path" :to="catalog.path(item.path)">
          <UiIcon :name="item.icon" :size="15" />{{ t(item.label) }}
        </router-link>
      </section>
      <section v-if="server.length">
        <strong>{{ t('sideNavigation.server') }}</strong>
        <router-link v-for="item in server" :key="item.path" :to="item.path">
          <UiIcon :name="item.icon" :size="15" />{{ t(item.label) }}
        </router-link>
      </section>
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
import { objectDestinations, serverDestinations } from '../lib/workspaceNavigation'
const { t } = useI18n()
const props = defineProps<{ db: string | null }>()
const session = useSessionStore()
const catalog = useCatalogStore()
const route = useRoute()
const caps = computed(() => session.sessionInfo?.caps)
const objects = computed(() => props.db ? objectDestinations(props.db, caps.value) : [])
const server = computed(() => serverDestinations(caps.value))
const advancedRoute = computed(() => ['db-relationships', 'db-objects', 'server-users', 'server-processes'].includes(String(route.name)))
</script>
<style scoped>
summary, a { display: flex; align-items: center; gap: 9px; padding: 6px 9px; min-height: 30px; box-sizing: border-box; border-radius: 6px; line-height: 1.3; font-size: 13px; color: var(--db-ink); }
summary { list-style: none; cursor: pointer; }
summary::-webkit-details-marker { display: none; }
summary svg, a svg { color: var(--db-muted); }
.disclosure { margin-left: auto; }
details[open] .disclosure { transform: rotate(90deg); }
nav { margin: 4px 0 4px 15px; padding-left: 6px; border-left: 1px solid var(--db-line); }
section + section { margin-top: 5px; }
strong { display: block; padding: 5px 8px 2px; font-size: 11px; font-weight: 600; color: var(--db-muted); }
a { text-decoration: none; overflow-wrap: anywhere; }
summary:hover, a:hover, .router-link-exact-active { background: var(--db-tint); color: var(--db-accent); }
.router-link-exact-active svg { color: var(--db-accent); }
</style>

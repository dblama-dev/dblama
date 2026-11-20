<template>
  <details v-if="db || caps?.users || caps?.monitor" class="advanced-navigation">
    <summary>{{ t('sideNavigation.more') }}</summary>
    <nav :aria-label="t('sideNavigation.more')">
      <section v-if="db">
        <strong>{{ t('sideNavigation.databaseObjects') }}</strong>
        <router-link :to="catalog.path(`/db/${encodeURIComponent(db)}/relationships`)">{{ t('relationships.title') }}</router-link>
        <router-link v-for="kind in kinds" :key="kind" :to="catalog.path(`/db/${encodeURIComponent(db)}/objects/${kind}`)">{{ t(`objects.${kind}`) }}</router-link>
      </section>
      <section v-if="caps?.users || caps?.monitor">
        <strong>{{ t('sideNavigation.server') }}</strong>
        <router-link v-if="caps?.users" to="/server/users">{{ t('admin.users') }}</router-link>
        <router-link v-if="caps?.monitor" to="/server/processes">{{ t('admin.monitor') }}</router-link>
      </section>
    </nav>
  </details>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
const { t } = useI18n()
defineProps<{ db: string | null }>()
const session = useSessionStore()
const catalog = useCatalogStore()
const caps = computed(() => session.sessionInfo?.caps)
const kinds = computed(() => (['views', 'triggers', 'routines', 'events'] as const).filter(kind => caps.value?.[kind]))
</script>
<style scoped>
.advanced-navigation summary { cursor: pointer; padding: 4px 0; }
.advanced-navigation nav, .advanced-navigation section { display: grid; gap: 3px; }
.advanced-navigation nav { padding: 6px 8px; gap: 10px; }
.advanced-navigation strong { font-size: 12px; }
a { display: block; color: inherit; text-decoration: none; overflow-wrap: anywhere; }
a:hover, .router-link-exact-active { text-decoration: underline; }
</style>

<template>
  <nav class="advanced-navigation">
    <router-link to="/server">{{ t('server.title') }}</router-link>
    <router-link v-if="caps?.users" to="/server/users">{{ t('admin.users') }}</router-link>
    <router-link v-if="caps?.monitor" to="/server/processes">{{ t('admin.monitor') }}</router-link>
    <template v-if="db">
      <router-link :to="catalog.path(`/db/${encodeURIComponent(db)}`)">{{ db }}</router-link>
      <router-link v-for="action in ['console','search','import','export']" :key="action" :to="catalog.path(`/db/${encodeURIComponent(db)}/${action}`)">{{ t(`navigationWorkflows.${action}`) }}</router-link>
      <router-link v-for="kind in kinds" :key="kind" :to="catalog.path(`/db/${encodeURIComponent(db)}/objects/${kind}`)">{{ t(`objects.${kind}`) }}</router-link>
    </template>
  </nav>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
const { t } = useI18n()
const route = useRoute()
const session = useSessionStore()
const catalog = useCatalogStore()
const caps = computed(() => session.sessionInfo?.caps)
const db = computed(() => typeof route.params.db === 'string' ? route.params.db : session.sessionInfo?.database ?? '')
const kinds = computed(() => (['views', 'triggers', 'routines', 'events'] as const).filter(kind => caps.value?.[kind]))
</script>
<style scoped>
.advanced-navigation { display: flex; flex-direction: column; gap: 8px; padding: 12px; border-top: 1px solid #8884; }
a { color: inherit; text-decoration: none; }
a:hover, .router-link-exact-active { text-decoration: underline; }
</style>

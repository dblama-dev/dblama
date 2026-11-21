<template>
  <section class="database-operations-view db-page tool-workspace">
    <ToolHeading icon="settings" :title="t('databaseOps.databaseActions')" />
    <div class="tool-card tool-card__body">
      <DatabaseActions :key="scope" :db="db" @renamed="renamed" @dropped="dropped">
        <template #heading><h2 class="database-name">{{ db }}</h2></template>
      </DatabaseActions>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import DatabaseActions from '../components/DatabaseActions.vue'
import ToolHeading from '../components/ToolHeading.vue'
import { useCatalogStore } from '../stores/catalog'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const catalog = useCatalogStore()
const db = computed(() => String(route.params.db))
const scope = computed(() => catalog.path(`/db/${encodeURIComponent(db.value)}`))

function renamed(name: string): void {
  void router.replace(catalog.path(`/db/${encodeURIComponent(name)}`))
}

function dropped(): void {
  void router.replace('/server')
}
</script>

<style scoped>
.database-name { font-family: var(--db-mono); overflow-wrap: anywhere; }
</style>

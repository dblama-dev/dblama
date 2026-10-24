<template>
  <div class="object-tree">
    <n-select
      size="small"
      :value="db"
      :options="dbOptions"
      :loading="dbsLoading"
      :placeholder="t('tree.selectDb')"
      @update:value="selectDb"
    />
    <n-input
      v-model:value="search"
      size="small"
      clearable
      :placeholder="t('tree.search')"
    />
    <n-alert v-if="dbsError" type="error" :title="t('tree.selectDb')">
      {{ dbsError }}
    </n-alert>
    <n-alert v-else-if="catalog.error" type="error" :title="t('tree.loadFailed')">
      {{ catalog.error }}
    </n-alert>
    <n-tree
      v-else
      block-line
      :data="treeData"
      :selected-keys="selectedKeys"
      :expanded-keys="expandedKeys"
      :disabled="catalog.loading"
      @update:expanded-keys="onExpand"
      @update:selected-keys="onSelect"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NInput, NSelect, NTree, type TreeOption } from 'naive-ui'

import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'

const { t } = useI18n()
const router = useRouter()
const catalog = useCatalogStore()
const session = useSessionStore()

const db = ref<string | null>(null)
const dbs = ref<string[]>([])
const dbsLoading = ref(false)
const dbsError = ref('')
const search = ref('')
const selectedKeys = ref<string[]>([])
// The database node starts expanded so tables are visible right away;
// the user can collapse it afterwards.
const expandedKeys = ref<string[]>([])

const dbOptions = computed(() => dbs.value.map((name) => ({ label: name, value: name })))

/** Loads the database list and preselects the session database, falling
 * back to the first listed database. Tables of the selection are fetched
 * through the catalog store. */
onMounted(async () => {
  dbsLoading.value = true
  dbsError.value = ''
  try {
    const payload = await api<{ databases: { name: string }[] }>('GET', '/api/databases')
    dbs.value = (payload.databases ?? []).map((d) => d.name)
  } catch (e) {
    dbsError.value = e instanceof Error ? e.message : String(e)
  } finally {
    dbsLoading.value = false
  }
  const initial = session.sessionInfo?.database
  db.value = initial && dbs.value.includes(initial) ? initial : (dbs.value[0] ?? null)
  expandedKeys.value = defaultExpandedKeys(db.value)
  if (db.value !== null) await catalog.fetchTables(db.value)
})

async function selectDb(next: string): Promise<void> {
  db.value = next
  selectedKeys.value = []
  expandedKeys.value = defaultExpandedKeys(next)
  await catalog.fetchTables(next)
}

/** The db node and its groups start expanded; unknown keys are ignored by
 * the tree (e.g. group:views when the database has no views). */
function defaultExpandedKeys(database: string | null): string[] {
  return database === null ? [] : [`db:${database}`, 'group:tables', 'group:views']
}

function onExpand(keys: string[]): void {
  expandedKeys.value = keys
}

const treeData = computed<TreeOption[]>(() => {
  if (db.value === null) return []
  const needle = search.value.trim().toLowerCase()
  const matches = (name: string): boolean => name.toLowerCase().includes(needle)
  const tables = catalog.tables
    .filter((table) => !table.isView && matches(table.name))
    .map((table) => ({ key: `table:${table.name}`, label: table.name, isLeaf: true }))
  const children: TreeOption[] = [
    {
      // The SQL console of this database; a plain leaf, no search filter.
      key: 'console',
      label: t('tree.console'),
      isLeaf: true,
    },
    {
      key: 'export',
      label: t('tree.export'),
      isLeaf: true,
    },
    {
      key: 'import',
      label: t('tree.import'),
      isLeaf: true,
    },
    {
      key: 'group:tables',
      label: t('tree.tables'),
      children: tables,
      // The "+" opens the structure editor on the _new placeholder to
      // create a table; stopPropagation keeps the click from selecting
      // or expanding the group node.
      suffix: () =>
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          title: t('tree.newTable'),
          'data-testid': 'tree-add-table',
          onClick: (e: MouseEvent) => {
            e.stopPropagation()
            void createTable()
          },
        }, { default: () => '+' }),
    },
  ]
  const views = catalog.tables
    .filter((table) => table.isView && matches(table.name))
    .map((table) => ({ key: `table:${table.name}`, label: table.name, isLeaf: true }))
  // The Views group appears only when the database actually has views.
  if (views.length > 0) {
    children.push({ key: 'group:views', label: t('tree.views'), children: views })
  }
  return [{ key: `db:${db.value}`, label: db.value, children }]
})

function onSelect(keys: string[]): void {
  const key = keys[keys.length - 1]
  if (key === undefined || db.value === null) return
  selectedKeys.value = [key]
  // Identifiers may contain %, ? or #; vue-router decodes params on read.
  if (key === 'console' || key === 'export' || key === 'import') {
    void router.push(`/db/${encodeURIComponent(db.value)}/${key}`)
    return
  }
  if (!key.startsWith('table:')) return
  const table = key.slice('table:'.length)
  void router.push(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent(table)}`)
}

/** Opens the create-table structure editor on the _new placeholder. */
function createTable(): void {
  if (db.value === null) return
  void router.push(`/db/${encodeURIComponent(db.value)}/table/${encodeURIComponent('_new')}?view=structure`)
}
</script>

<style scoped>
.object-tree {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}
</style>

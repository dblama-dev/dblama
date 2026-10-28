<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotification } from 'naive-ui'
import { NButton, NInput, NInputNumber, NSelect } from 'naive-ui'

import { useCatalogStore } from '../../stores/catalog'
import { api, ApiError } from '../../api/client'
import type { Index, IndexColumn } from '../../api/types'

// Index editor: a local editable copy of the table's indexes. Saving posts
// the FULL desired list to POST /tables/{t}/indexes — the backend diffs it
// against the current state and drops/adds what changed.

interface EditableIndexColumn {
  name: string
  subPart: number
}

interface EditableIndex {
  key: number
  name: string
  type: string
  columns: EditableIndexColumn[]
}

const props = defineProps<{
  db: string
  table: string
  indexes: Index[]
  columnNames: string[]
  draft?: boolean
}>()

const emit = defineEmits<{ saved: []; 'update:indexes': [indexes: Index[]]; validity: [valid: boolean] }>()

const { t } = useI18n()
const notification = useNotification()
const catalog = useCatalogStore()

const INDEX_TYPES = ['PRIMARY', 'UNIQUE', 'INDEX', 'FULLTEXT', 'SPATIAL']

const rows = ref<EditableIndex[]>([])
let keySeq = 0

// Rebuild the local copy whenever the parent hands over a fresh definition
// (after load and after every successful save refetch).
watch(
  () => props.indexes,
  (next) => {
    if (JSON.stringify(next ?? []) === JSON.stringify(rows.value.map(toIndex))) return
    rows.value = (next ?? []).map((idx) => ({
      key: ++keySeq,
      name: idx.name,
      type: idx.type,
      columns: idx.columns.map((c) => ({ name: c.name, subPart: c.subPart ?? 0 })),
    }))
  },
  { immediate: true },
)

const typeOptions = computed(() =>
  INDEX_TYPES.map((type) => ({
    label: type,
    value: type,
    // At most one PRIMARY per table.
    disabled: type === 'PRIMARY' && rows.value.some((r) => r.type === 'PRIMARY'),
  })),
)

const columnOptions = computed(() =>
  props.columnNames.map((name) => ({ label: name, value: name })),
)

function addIndex(): void {
  rows.value.push({ key: ++keySeq, name: '', type: 'INDEX', columns: [] })
}

function removeIndex(index: EditableIndex): void {
  rows.value = rows.value.filter((r) => r !== index)
}

function onColumnsChanged(index: EditableIndex, names: string[]): void {
  // Keep the sub-part of columns that stay selected; new ones default to
  // the full column (0).
  const previous = new Map(index.columns.map((c) => [c.name, c.subPart]))
  index.columns = names.map((name) => ({ name, subPart: previous.get(name) ?? 0 }))
}

function toIndex(index: EditableIndex): Index {
  const columns: IndexColumn[] = index.columns.map((c) => ({
    name: c.name,
    ...(c.subPart > 0 ? { subPart: c.subPart } : {}),
  }))
  // PRIMARY has no name of its own.
  return index.type === 'PRIMARY'
    ? { name: 'PRIMARY', type: 'PRIMARY', columns }
    : { name: index.name, type: index.type, columns }
}

function canSave(): boolean {
  return rows.value.every(
    (r) =>
      r.columns.length > 0 &&
      r.columns.every((c) => props.columnNames.includes(c.name)) &&
      (r.type === 'PRIMARY' || r.name.trim() !== ''),
  )
}

watch(rows, () => {
  if (props.draft) emit('update:indexes', rows.value.map(toIndex))
}, { deep: true })
watch(() => canSave(), valid => emit('validity', valid), { immediate: true })

const saving = ref(false)

async function save(): Promise<void> {
  saving.value = true
  try {
    await api(
      'POST',
      catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}/indexes`),
      rows.value.map(toIndex),
    )
    notification.success({ content: t('structure.saved'), duration: 3000 })
    emit('saved')
  } catch (err) {
    const message = err instanceof ApiError ? err.message : String(err)
    notification.error({ content: message, duration: 5000 })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="index-editor">
    <div
      v-for="index in rows"
      :key="index.key"
      class="index-editor__row"
      :data-testid="'index-row'"
    >
      <label class="struct-field struct-field--indexname">
        <span class="struct-field__label">{{ t('structure.name') }}</span>
        <NInput
          v-model:value="index.name"
          :disabled="index.type === 'PRIMARY'"
          :placeholder="t('structure.name')"
        />
      </label>
      <label class="struct-field struct-field--indextype">
        <span class="struct-field__label">{{ t('structure.indexType') }}</span>
        <NSelect v-model:value="index.type" :options="typeOptions" />
      </label>
      <label class="struct-field struct-field--indexcolumns">
        <span class="struct-field__label">{{ t('structure.indexColumns') }}</span>
        <NSelect
          :value="index.columns.map((c) => c.name)"
          :options="columnOptions"
          multiple
          @update:value="onColumnsChanged(index, $event)"
        />
      </label>
      <div
        v-for="col in index.columns"
        :key="col.name"
        class="index-editor__subpart"
      >
        <span class="index-editor__subpart-name">{{ col.name }}</span>
        <NInputNumber
          v-model:value="col.subPart"
          :min="0"
          size="small"
        >
          <template #suffix>{{ t('structure.subPartHint') }}</template>
        </NInputNumber>
      </div>
      <NButton
        size="small"
        quaternary
        @click="removeIndex(index)"
      >
        {{ t('structure.removeIndex') }}
      </NButton>
    </div>
    <div class="index-editor__actions">
      <NButton
        size="small"
        @click="addIndex"
      >
        {{ t('structure.addIndex') }}
      </NButton>
      <NButton
        v-if="!draft"
        size="small"
        type="primary"
        :disabled="!canSave() || saving"
        :loading="saving"
        data-testid="index-save"
        @click="save"
      >
        {{ t('structure.saveIndexes') }}
      </NButton>
    </div>
  </section>
</template>

<style scoped>
.index-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.index-editor__row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
  padding: 8px;
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 4px;
}

.index-editor__row .struct-field {
  min-width: 160px;
}

.index-editor__row .struct-field--indexcolumns {
  flex: 1;
  min-width: 240px;
}

.index-editor__subpart {
  display: flex;
  align-items: center;
  gap: 4px;
}

.index-editor__subpart-name {
  font-size: 12px;
  opacity: 0.7;
}

.index-editor__actions {
  display: flex;
  gap: 8px;
}
</style>

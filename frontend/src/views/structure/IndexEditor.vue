<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotification } from '../../ui'
import { UiButton, UiInput, UiInputNumber, UiSelect } from '../../ui'

import UiIcon from '../../components/UiIcon.vue'
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

const emit = defineEmits<{ saved: [submitted?: Index[]]; 'update:indexes': [indexes: Index[]]; validity: [valid: boolean] }>()

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
  INDEX_TYPES.filter(type => !catalog.schemaAware || (type !== 'FULLTEXT' && type !== 'SPATIAL')).map((type) => ({
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
  emit('update:indexes', rows.value.map(toIndex))
}, { deep: true })
watch(() => canSave(), valid => emit('validity', valid), { immediate: true })

const saving = ref(false)

async function save(): Promise<void> {
  saving.value = true
  const submitted = rows.value.map(toIndex)
  try {
    await api(
      'POST',
      catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}/indexes`),
      submitted,
    )
    notification.success({ content: t('structure.saved'), duration: 3000 })
    emit('saved', submitted)
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
        <UiInput
          v-model:value="index.name"
          :disabled="index.type === 'PRIMARY'"
          :placeholder="t('structure.name')"
        />
      </label>
      <label class="struct-field struct-field--indextype">
        <span class="struct-field__label">{{ t('structure.indexType') }}</span>
        <UiSelect v-model:value="index.type" :options="typeOptions" />
      </label>
      <label class="struct-field struct-field--indexcolumns">
        <span class="struct-field__label">{{ t('structure.indexColumns') }}</span>
        <UiSelect
          :value="index.columns.map((c) => c.name)"
          :options="columnOptions"
          multiple
          @update:value="onColumnsChanged(index, $event)"
        />
      </label>
      <UiButton class="index-editor__remove" size="small" quaternary
        :aria-label="`${t('structure.removeIndex')} ${index.name || t('structure.indexes')}`" @click="removeIndex(index)">
        <UiIcon name="x" :size="15" />{{ t('structure.removeIndex') }}
      </UiButton>
      <fieldset v-if="!catalog.schemaAware && index.columns.length" class="index-editor__prefixes">
        <legend>{{ t('structure.indexPrefix') }}</legend>
        <label v-for="col in index.columns" :key="col.name" class="index-editor__subpart">
          <span>{{ col.name }}</span>
          <UiInputNumber v-model:value="col.subPart" :min="0" :precision="0" size="small" />
        </label>
        <small>{{ t('structure.subPartHint') }}</small>
      </fieldset>
    </div>
    <div class="index-editor__actions">
      <UiButton
        size="small"
        @click="addIndex"
      >
        {{ t('structure.addIndex') }}
      </UiButton>
      <UiButton
        v-if="!draft"
        size="small"
        type="primary"
        :disabled="!canSave() || saving"
        :loading="saving"
        data-testid="index-save"
        @click="save"
      >
        {{ t('structure.saveIndexes') }}
      </UiButton>
    </div>
  </section>
</template>

<style scoped>
.index-editor { display: grid; gap: 12px; container-type: inline-size; }
.index-editor__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(7.5rem, .65fr) minmax(0, 1.25fr) auto;
  align-items: end;
  gap: 12px;
  padding: 14px;
  background: var(--db-sidebar);
  border: 1px solid var(--db-line);
  border-radius: 8px;
}
.struct-field { display: grid; gap: 4px; min-width: 0; }
.struct-field__label, .index-editor__prefixes legend, .index-editor__subpart span { color: var(--db-muted); font-size: 12px; }
.index-editor__remove { justify-self: end; }
.index-editor__prefixes {
  grid-column: 1 / -1;
  display: flex;
  align-items: end;
  flex-wrap: wrap;
  gap: 6px 14px;
  min-width: 0;
  margin: 0;
  padding: 2px 0 0;
  border: 0;
}
.index-editor__prefixes legend { margin-bottom: 4px; padding: 0; font-weight: 550; }
.index-editor__subpart { display: grid; width: 8rem; gap: 3px; }
.index-editor__subpart span { overflow-wrap: anywhere; }
.index-editor__prefixes small { align-self: end; padding: 6px 0; color: var(--db-muted); font-size: 12px; }
.index-editor__actions { display: flex; flex-wrap: wrap; gap: 8px; }
@container (max-width: 44rem) {
  .index-editor__row { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  .struct-field--indexcolumns { grid-column: 1 / -1; }
  .index-editor__remove { grid-column: 1 / -1; justify-self: start; }
  .index-editor__prefixes { grid-row: 3; }
}
</style>

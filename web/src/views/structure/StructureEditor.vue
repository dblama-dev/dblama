<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotification } from 'naive-ui'
import { NButton, NCheckbox, NInput, NRadio, NSelect } from 'naive-ui'

import { api, ApiError } from '../../api/client'
import { useCatalogStore } from '../../stores/catalog'
import type { ColumnDef, ForeignKey, Index, TableDef, TableOptions } from '../../api/types'
import IndexEditor from './IndexEditor.vue'
import OptionsEditor from './OptionsEditor.vue'

// Table structure editor: columns, indexes and table options. Columns and
// options go out together in one PUT of the whole TableDef; indexes have
// their own endpoint (POST .../indexes) and save button. The backend
// SaveTable contract:
//   - oldName != "" + type != ""  → MODIFY / CHANGE (oldName != name)
//   - oldName == "" + type != ""  → ADD (placement from position:
//     "FIRST" or "AFTER <previous column>")
//   - oldName != "" + type == ""  → DROP
// The read side sets oldName = name, so an untouched round-trip only
// produces MODIFY statements. Every row carries a position (FIRST or
// AFTER the previous displayed column), so reordering existing columns
// reaches the server too.
// Create mode: when the route's table is the literal "_new" no
// definition is fetched; saving PUTs the whole TableDef to
// /tables/_new (an empty table name makes the backend CREATE it) and
// the created name is reported back via the created event.

interface EditableColumn {
  key: number
  name: string
  /** Name the column has in the loaded definition; null for rows added
   * in this session (they are sent without oldName → ADD). */
  originalName: string | null
  type: string
  length: string
  unsigned: boolean
  nullable: boolean
  defaultNull: boolean
  isSqlExpression: boolean
  defaultValue: string
  autoIncrement: boolean
  onUpdate: string
  generated: '' | 'VIRTUAL' | 'STORED'
  generatedExpression: string
  collation: string
  comment: string
  enumValues: string
  markedForDeletion: boolean
}

const props = defineProps<{ db: string; table: string }>()
const emit = defineEmits<{ renamed: [name: string]; created: [name: string] }>()

const { t } = useI18n()
const notification = useNotification()
const catalog = useCatalogStore()

const COMMON_TYPES = [
  'int',
  'bigint',
  'varchar',
  'text',
  'longtext',
  'decimal',
  'datetime',
  'timestamp',
  'date',
  'enum',
  'json',
  'char',
  'float',
  'double',
  'blob',
]

const NUMERIC_TYPES = new Set([
  'int',
  'bigint',
  'smallint',
  'tinyint',
  'mediumint',
  'decimal',
  'float',
  'double',
  'year',
])

const ENUM_TYPES = new Set(['enum', 'set'])

const typeOptions = COMMON_TYPES.map((type) => ({ label: type, value: type }))
const generatedOptions = [
  { label: t('structure.generatedNone'), value: '' },
  { label: 'VIRTUAL', value: 'VIRTUAL' },
  { label: 'STORED', value: 'STORED' },
]

const path = computed(
  () =>
    catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}`),
)

// Create mode: the route addresses the literal "_new" placeholder, no
// definition exists yet.
const createMode = computed(() => props.table === '_new')

const loading = ref(false)
const saving = ref(false)
const tableName = ref(props.table === '_new' ? '' : props.table)
const rows = ref<EditableColumn[]>([])
const options = ref<TableOptions>({})
const indexes = ref<Index[]>([])
const foreignKeys = ref<ForeignKey[]>([])
let keySeq = 0

function fromDef(c: ColumnDef): EditableColumn {
  const generated = (c.generated ?? '') as EditableColumn['generated']
  let defaultNull = false
  let isSql = false
  let defaultValue = ''
  if (!generated && c.default) {
    if (c.default.isSqlExpression && c.default.value === 'NULL') {
      // The model's way to express an explicit DEFAULT NULL.
      defaultNull = true
    } else {
      defaultValue = c.default.value
      isSql = c.default.isSqlExpression ?? false
    }
  }
  return {
    key: ++keySeq,
    name: c.name,
    originalName: c.oldName ?? c.name,
    type: c.type,
    length: c.length ?? '',
    unsigned: c.unsigned ?? false,
    nullable: c.nullable,
    defaultNull,
    isSqlExpression: isSql,
    defaultValue,
    autoIncrement: c.autoIncrement ?? false,
    onUpdate: c.onUpdate ?? '',
    generated,
    generatedExpression: generated ? (c.default?.value ?? '') : '',
    collation: c.collation ?? '',
    comment: c.comment ?? '',
    enumValues: (c.enumValues ?? []).join(', '),
    markedForDeletion: false,
  }
}

async function load(): Promise<void> {
  if (createMode.value) {
    // Create mode owns no definition: reset everything so reusing this
    // component after another table's structure view (vue-router reuses
    // the component instance) starts from a blank editor instead of the
    // previous table's state.
    resetEditor()
    return
  }
  loading.value = true
  try {
    const def = await api<TableDef>('GET', path.value)
    tableName.value = def.newName || props.table
    rows.value = (def.columns ?? []).map(fromDef)
    options.value = { ...def.options }
    indexes.value = def.indexes ?? []
    foreignKeys.value = def.foreignKeys ?? []
  } catch (err) {
    notifyError(err)
  } finally {
    loading.value = false
  }
}

function resetEditor(): void {
  tableName.value = ''
  rows.value = []
  options.value = {}
  indexes.value = []
  foreignKeys.value = []
}

watch(
  () => [props.db, props.table, catalog.schema],
  () => void load(),
  { immediate: true },
)

function notifyError(err: unknown): void {
  const message = err instanceof ApiError ? err.message : String(err)
  notification.error({ content: message, duration: 5000 })
}

// --- Row manipulation -----------------------------------------------------

function addColumn(): void {
  rows.value.push({
    key: ++keySeq,
    name: '',
    originalName: null,
    type: '',
    length: '',
    unsigned: false,
    nullable: true,
    defaultNull: false,
    isSqlExpression: false,
    defaultValue: '',
    autoIncrement: false,
    onUpdate: '',
    generated: '',
    generatedExpression: '',
    collation: '',
    comment: '',
    enumValues: '',
    markedForDeletion: false,
  })
}

function moveRow(index: number, delta: number): void {
  const target = index + delta
  if (target < 0 || target >= rows.value.length) return
  const copy = [...rows.value]
  ;[copy[index], copy[target]] = [copy[target], copy[index]]
  rows.value = copy
}

function toggleDeletion(row: EditableColumn): void {
  if (!row.originalName) {
    // A row that was never saved is simply removed.
    rows.value = rows.value.filter((r) => r !== row)
    return
  }
  row.markedForDeletion = !row.markedForDeletion
}

function setAutoIncrement(row: EditableColumn, value: boolean): void {
  // At most one AUTO_INCREMENT column per table.
  for (const r of rows.value) {
    r.autoIncrement = r === row && value
  }
}

function isNumeric(row: EditableColumn): boolean {
  return NUMERIC_TYPES.has(row.type.trim().toLowerCase())
}

function isEnumish(row: EditableColumn): boolean {
  return ENUM_TYPES.has(row.type.trim().toLowerCase())
}

function onGeneratedChanged(row: EditableColumn, value: '' | 'VIRTUAL' | 'STORED'): void {
  row.generated = value
  if (value) {
    // Generated columns cannot carry NULL/NOT NULL or a plain default
    // (the backend rejects NOT NULL generated columns cross-flavor).
    row.nullable = false
  }
}

// --- Save ----------------------------------------------------------------

const canSave = computed(
  () =>
    tableName.value.trim() !== '' &&
    // Create mode needs at least one column (the backend rejects an
    // empty CREATE TABLE anyway); alter mode with zero surviving rows
    // is left to the backend to report.
    (!createMode.value || rows.value.some((r) => !r.markedForDeletion)) &&
    rows.value
      .filter((r) => !r.markedForDeletion)
      .every((r) => r.name.trim() !== '' && r.type.trim() !== ''),
)

function toColumnDef(row: EditableColumn, after: string): ColumnDef {
  const col: ColumnDef = {
    name: row.name,
    type: row.type,
    nullable: row.generated ? false : row.nullable,
  }
  if (row.originalName) col.oldName = row.originalName
  const length = row.length.trim()
  if (length !== '') col.length = length
  if (row.unsigned) col.unsigned = true

  if (row.generated) {
    // The expression travels in Default.Value as an SQL expression; the
    // save side renders it as AS (expr).
    col.generated = row.generated
    col.default = { value: row.generatedExpression, isSqlExpression: true }
  } else if (row.defaultNull) {
    col.default = { value: 'NULL', isSqlExpression: true }
  } else if (row.isSqlExpression) {
    col.default = { value: row.defaultValue, isSqlExpression: true }
  } else if (row.defaultValue !== '') {
    col.default = { value: row.defaultValue, isSqlExpression: false }
  }

  if (row.autoIncrement) col.autoIncrement = true
  const onUpdate = row.onUpdate.trim()
  if (onUpdate !== '') col.onUpdate = onUpdate
  if (isEnumish(row)) {
    const values = row.enumValues
      .split(',')
      .map((v) => v.trim())
      .filter((v) => v !== '')
    if (values.length > 0) col.enumValues = values
  }
  const collation = row.collation.trim()
  if (collation !== '') col.collation = collation
  if (row.comment !== '') col.comment = row.comment

  // Every column (not only new ones) carries its placement from the
  // desired display order — FIRST for the first row, AFTER the previous
  // non-deleted row otherwise. For an unchanged MODIFY the clause is a
  // harmless, idempotent no-op; for a moved column it is what makes the
  // reorder reach the server at all (buildAlterTable takes the placement
  // from the position field, not from the array order).
  if (!catalog.schemaAware) col.position = after || 'FIRST'
  return col
}

function buildColumns(): ColumnDef[] {
  const result: ColumnDef[] = []
  let after = ''
  for (const row of rows.value) {
    if (row.markedForDeletion) {
      if (row.originalName) {
        result.push({ name: row.originalName, oldName: row.originalName, type: '' })
      }
      // A dropped column is no anchor for the next ADD.
      continue
    }
    result.push(toColumnDef(row, after))
    if (row.name.trim() !== '') after = row.name
  }
  return result
}

async function save(): Promise<void> {
  saving.value = true
  try {
    if (createMode.value) {
      // The empty table name is the backend's CREATE TABLE contract;
      // the name of the new table travels in newName.
      await api('PUT', path.value, {
        newName: tableName.value,
        columns: buildColumns(),
        indexes: indexes.value,
        foreignKeys: foreignKeys.value,
        options: options.value,
      })
      notification.success({ content: t('structure.saved'), duration: 3000 })
      // The parent navigates to the new table (and invalidates the
      // catalog); the db/table watch then loads the fresh definition.
      emit('created', tableName.value)
      return
    }
    await api('PUT', path.value, {
      newName: tableName.value,
      columns: buildColumns(),
      // Index changes have their own endpoint; SaveTable ignores index
      // changes on an existing table, so the list is sent as loaded to
      // keep the CREATE path (and the payload shape) consistent.
      indexes: indexes.value,
      // Foreign key editing is out of scope for now (phase 5).
      foreignKeys: foreignKeys.value,
      options: options.value,
    })
    notification.success({ content: t('structure.saved'), duration: 3000 })
    await catalog.invalidate(props.db)
    if (tableName.value !== props.table) {
      // The route still points at the old name; the parent navigates and
      // the db/table watch refetches the definition under the new name.
      emit('renamed', tableName.value)
      return
    }
    await load()
  } catch (err) {
    notifyError(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="structure-editor">
    <div
      v-if="loading"
      class="structure-editor__loading"
    >
      {{ t('structure.loading') }}
    </div>
    <template v-else>
      <div class="structure-editor__header">
        <label class="struct-field struct-field--table-name">
          <span class="struct-field__label">{{ t('structure.tableName') }}</span>
          <NInput
            v-model:value="tableName"
            :placeholder="t('structure.tableName')"
          />
        </label>
        <NButton
          type="primary"
          :disabled="!canSave || saving"
          :loading="saving"
          data-testid="struct-save"
          @click="save"
        >
          {{ t('structure.save') }}
        </NButton>
      </div>

      <h3 class="structure-editor__title">
        {{ t('structure.columns') }}
      </h3>
      <div class="structure-editor__columns">
        <div
          v-for="(row, index) in rows"
          :key="row.key"
          class="struct-row"
          :class="{ 'struct-row--deleted': row.markedForDeletion }"
          data-testid="struct-row"
        >
          <label
            class="struct-field struct-field--name"
            :title="row.originalName ?? ''"
          >
            <span class="struct-field__label">{{ t('structure.name') }}</span>
            <NInput
              v-model:value="row.name"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label class="struct-field struct-field--type">
            <span class="struct-field__label">{{ t('structure.type') }}</span>
            <NSelect
              v-model:value="row.type"
              :options="typeOptions"
              :disabled="row.markedForDeletion"
              filterable
              tag
              :placeholder="t('structure.type')"
            />
          </label>
          <label
            v-if="!isEnumish(row)"
            class="struct-field struct-field--length"
          >
            <span class="struct-field__label">{{ t('structure.length') }}</span>
            <NInput
              v-model:value="row.length"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label
            v-if="isEnumish(row)"
            class="struct-field struct-field--enumvalues"
          >
            <span class="struct-field__label">{{ t('structure.enumValues') }}</span>
            <NInput
              v-model:value="row.enumValues"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label
            v-if="isNumeric(row)"
            class="struct-field struct-field--unsigned"
          >
            <span class="struct-field__label">{{ t('structure.unsigned') }}</span>
            <NCheckbox
              :checked="row.unsigned"
              :disabled="row.markedForDeletion"
              @update:checked="row.unsigned = $event"
            />
          </label>
          <label class="struct-field struct-field--nullable">
            <span class="struct-field__label">{{ t('structure.nullable') }}</span>
            <NCheckbox
              :checked="row.nullable"
              :disabled="row.generated !== '' || row.markedForDeletion"
              @update:checked="row.nullable = $event"
            />
          </label>
          <label
            v-if="!row.generated"
            class="struct-field struct-field--default"
          >
            <span class="struct-field__label">{{ t('structure.defaultValue') }}</span>
            <NInput
              v-model:value="row.defaultValue"
              :disabled="row.defaultNull || row.markedForDeletion"
            />
          </label>
          <label
            v-if="!row.generated"
            class="struct-field struct-field--defaultnull"
          >
            <span class="struct-field__label">{{ t('structure.nullDefault') }}</span>
            <NCheckbox
              :checked="row.defaultNull"
              :disabled="row.markedForDeletion"
              @update:checked="
                (v) => {
                  row.defaultNull = v
                  if (v) row.isSqlExpression = false
                }
              "
            />
          </label>
          <label
            v-if="!row.generated"
            class="struct-field struct-field--defaultsql"
          >
            <span class="struct-field__label">{{ t('structure.sqlExpression') }}</span>
            <NCheckbox
              :checked="row.isSqlExpression"
              :disabled="row.defaultNull || row.markedForDeletion"
              @update:checked="
                (v) => {
                  row.isSqlExpression = v
                  if (v) row.defaultNull = false
                }
              "
            />
          </label>
          <label
            v-else
            class="struct-field struct-field--genexpr"
          >
            <span class="struct-field__label">{{ t('structure.generatedExpression') }}</span>
            <NInput
              v-model:value="row.generatedExpression"
              :placeholder="t('structure.generatedHint')"
            />
          </label>
          <label class="struct-field struct-field--autoinc">
            <span class="struct-field__label">{{ t('structure.autoIncrement') }}</span>
            <NRadio
              :checked="row.autoIncrement"
              :disabled="row.generated !== '' || row.markedForDeletion"
              @update:checked="setAutoIncrement(row, $event)"
            />
          </label>
          <label class="struct-field struct-field--onupdate">
            <span class="struct-field__label">{{ t('structure.onUpdate') }}</span>
            <NInput
              v-model:value="row.onUpdate"
              :disabled="row.markedForDeletion"
              :placeholder="t('structure.onUpdateHint')"
            />
          </label>
          <label class="struct-field struct-field--generated">
            <span class="struct-field__label">{{ t('structure.generated') }}</span>
            <NSelect
              :value="row.generated"
              :options="generatedOptions"
              :disabled="row.markedForDeletion"
              @update:value="onGeneratedChanged(row, $event as EditableColumn['generated'])"
            />
          </label>
          <label class="struct-field struct-field--collation">
            <span class="struct-field__label">{{ t('structure.collation') }}</span>
            <NInput
              v-model:value="row.collation"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label class="struct-field struct-field--comment">
            <span class="struct-field__label">{{ t('structure.comment') }}</span>
            <NInput
              v-model:value="row.comment"
              :disabled="row.markedForDeletion"
            />
          </label>
          <div class="struct-row__buttons">
            <NButton
              size="small"
              quaternary
              :disabled="index === 0"
              v-if="!catalog.schemaAware"
              data-testid="col-up"
              :title="t('structure.moveUp')"
              @click="moveRow(index, -1)"
            >
              ↑
            </NButton>
            <NButton
              size="small"
              quaternary
              :disabled="index === rows.length - 1"
              v-if="!catalog.schemaAware"
              data-testid="col-down"
              :title="t('structure.moveDown')"
              @click="moveRow(index, 1)"
            >
              ↓
            </NButton>
            <NButton
              size="small"
              quaternary
              type="error"
              data-testid="col-delete"
              :title="row.markedForDeletion ? t('structure.restore') : t('structure.delete')"
              @click="toggleDeletion(row)"
            >
              ✕
            </NButton>
          </div>
          <div
            v-if="row.generated"
            class="struct-row__hint"
          >
            {{ t('structure.generatedHint') }}
          </div>
        </div>
        <NButton
          size="small"
          data-testid="struct-add"
          @click="addColumn"
        >
          {{ t('structure.addColumn') }}
        </NButton>
      </div>

      <h3
        v-if="!createMode"
        class="structure-editor__title"
      >
        {{ t('structure.indexes') }}
      </h3>
      <!-- The indexes endpoint only exists for a saved table; on create
           the inline indexes list of the PUT body is used instead. -->
      <IndexEditor
        v-if="!createMode"
        :db="db"
        :table="table"
        :indexes="indexes"
        :column-names="rows.filter((r) => !r.markedForDeletion).map((r) => r.name)"
        @saved="load"
      />

      <h3 class="structure-editor__title">
        {{ t('structure.options') }}
      </h3>
      <OptionsEditor
        v-model:options="options"
      />

      <template v-if="foreignKeys.length > 0">
        <h3 class="structure-editor__title">
          {{ t('structure.foreignKeys') }}
        </h3>
        <div class="structure-editor__fks">
          <div
            v-for="fk in foreignKeys"
            :key="fk.name"
            class="structure-editor__fk"
          >
            {{ fk.name }}: ({{ fk.columns.join(', ') }}) → {{ fk.refTable }}
            ({{ fk.refColumns.join(', ') }})
          </div>
          <div class="structure-editor__fk-hint">
            {{ t('structure.foreignKeysReadonly') }}
          </div>
        </div>
      </template>
    </template>
  </section>
</template>

<style scoped>
.structure-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.structure-editor__header {
  display: flex;
  align-items: flex-end;
  gap: 12px;
}

.structure-editor__header .struct-field--table-name {
  max-width: 320px;
}

.structure-editor__title {
  margin: 12px 0 0;
  font-size: 14px;
}

.structure-editor__columns {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.struct-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
  padding: 8px;
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 4px;
  width: 100%;
}

.struct-row--deleted .struct-field__label,
.struct-row--deleted :deep(.n-input__input-el) {
  text-decoration: line-through;
  opacity: 0.5;
}

.struct-row .struct-field {
  min-width: 130px;
}

.struct-row__buttons {
  display: flex;
  gap: 4px;
}

.struct-row__hint {
  width: 100%;
  font-size: 12px;
  opacity: 0.6;
}

.structure-editor__fks {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
}

.structure-editor__fk-hint {
  font-size: 12px;
  opacity: 0.6;
}
</style>

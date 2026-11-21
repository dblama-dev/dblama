<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { routerKey } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useNotification } from '../../ui'
import { UiButton, UiCheckbox, UiInput, UiSelect } from '../../ui'

import { api, ApiError } from '../../api/client'
import { useCatalogStore } from '../../stores/catalog'
import type { ColumnDef, ForeignKey, Index, TableDef, TableOptions } from '../../api/types'
import IndexEditor from './IndexEditor.vue'
import UiIcon from '../../components/UiIcon.vue'
import { useDatabaseMetadata } from '../../stores/metadata'
import ForeignKeyEditor from './ForeignKeyEditor.vue'
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
  defaultMode: 'none' | 'literal' | 'null' | 'expression'
  defaultValue: string
  autoIncrement: boolean
  savedSequence?: 'serial' | 'identity'
  onUpdate: string
  generated: '' | 'VIRTUAL' | 'STORED'
  generatedExpression: string
  collation: string
  comment: string
  enumValues: string[]
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

const metadata = useDatabaseMetadata(() => props.db)
const defaultOptions = computed(() => [
  { value: 'none', label: t('structure.noDefault') },
  { value: 'literal', label: t('structure.literalDefault') },
  { value: 'null', label: 'NULL' },
  { value: 'expression', label: t('structure.sqlExpression') },
])
const typeOptions = computed(() => metadata.data?.columnTypes.map(type => ({label:type.name,value:type.name})) ?? COMMON_TYPES.map(type => ({label:type,value:type})))
const generatedOptions = computed(() => [
  { label: t('structure.generatedNone'), value: '' },
  ...(!catalog.schemaAware ? [{ label: 'VIRTUAL', value: 'VIRTUAL' }] : []),
  { label: 'STORED', value: 'STORED' },
])

const path = computed(
  () =>
    catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}`),
)

// Create mode: the route addresses the literal "_new" placeholder, no
// definition exists yet.
const createMode = computed(() => props.table === '_new')
const canPlaceColumns = computed(() => createMode.value || !catalog.schemaAware)
const sections = [
  { name: 'columns', icon: 'columns' },
  { name: 'indexes', icon: 'key' },
  { name: 'foreignKeys', icon: 'network' },
  { name: 'options', icon: 'settings' },
] as const
const currentSection = ref<(typeof sections)[number]['name']>('columns')
const columnsPanel = ref<HTMLElement | null>(null)

const loading = ref(false)
const saving = ref(false)
const tableName = ref(props.table === '_new' ? '' : props.table)
const rows = ref<EditableColumn[]>([])
const options = ref<TableOptions>({})
const indexes = ref<Index[]>([])
const indexesValid = ref(true)
const foreignKeys = ref<ForeignKey[]>([])
let keySeq = 0
let loadRequest = 0
const columnBaseline = ref(''), indexBaseline=ref(''), fkBaseline=ref('')
const columnsSnapshot=()=>JSON.stringify({name:tableName.value,rows:rows.value.map(row=>({...row,key:0})),options:options.value})
const dirtyColumns=computed(()=>columnBaseline.value!=='' && columnsSnapshot()!==columnBaseline.value)
const dirtyIndexes=computed(()=>indexBaseline.value!=='' && JSON.stringify(indexes.value)!==indexBaseline.value)
const dirtyFKs=computed(()=>fkBaseline.value!=='' && JSON.stringify(foreignKeys.value)!==fkBaseline.value)
const dirty=computed(()=>dirtyColumns.value||dirtyIndexes.value||dirtyFKs.value)
const router=inject(routerKey,undefined)
const removeGuard=router?.beforeEach((to,from)=>{
  if(to.fullPath===from.fullPath || !dirty.value) return true
  return window.confirm(t('navigationWorkflows.discard'))
})
function beforeUnload(event:BeforeUnloadEvent):void {if(dirty.value){event.preventDefault();event.returnValue=''}}
window.addEventListener('beforeunload',beforeUnload)
onBeforeUnmount(()=>{removeGuard?.();window.removeEventListener('beforeunload',beforeUnload)})
async function snapshotAll():Promise<void>{await nextTick();columnBaseline.value=columnsSnapshot();indexBaseline.value=JSON.stringify(indexes.value);fkBaseline.value=JSON.stringify(foreignKeys.value)}
async function indexesSaved(submitted: Index[] = indexes.value):Promise<void>{indexBaseline.value=JSON.stringify(submitted);await catalog.invalidate(props.db)}
async function fksSaved():Promise<void>{fkBaseline.value=JSON.stringify(foreignKeys.value);await catalog.invalidate(props.db)}

function fromDef(c: ColumnDef): EditableColumn {
  const generated = (c.generated ?? '') as EditableColumn['generated']
  let defaultMode: EditableColumn['defaultMode'] = 'none'
  if (!generated && c.default) {
    defaultMode = c.default.isSqlExpression ? 'expression' : 'literal'
    if (c.default.isSqlExpression && c.default.value === 'NULL') defaultMode = 'null'
  }
  return {
    key: ++keySeq,
    name: c.name,
    originalName: c.oldName ?? c.name,
    type: c.type,
    length: c.length ?? '',
    unsigned: c.unsigned ?? false,
    nullable: c.nullable,
    defaultMode,
    defaultValue: defaultMode === 'literal' || defaultMode === 'expression' ? c.default!.value : '',
    autoIncrement: c.autoIncrement ?? false,
    savedSequence: c.autoIncrement ? c.default?.value.startsWith('nextval(') ? 'serial' : 'identity' : undefined,
    onUpdate: c.onUpdate ?? '',
    generated,
    generatedExpression: generated ? (c.default?.value ?? '') : '',
    collation: c.collation ?? '',
    comment: c.comment ?? '',
    enumValues: [...(c.enumValues ?? [])],
    markedForDeletion: false,
  }
}

async function load(columnsOnly = false): Promise<void> {
  const request=++loadRequest
  const requestedPath=path.value
  if (createMode.value) {
    // Create mode owns no definition: reset everything so reusing this
    // component after another table's structure view (vue-router reuses
    // the component instance) starts from a blank editor instead of the
    // previous table's state.
    resetEditor()
    await snapshotAll()
    return
  }
  loading.value = true
  try {
    const def = await api<TableDef>('GET', requestedPath)
    if(request!==loadRequest || requestedPath!==path.value)return
    tableName.value = def.newName || props.table
    rows.value = (def.columns ?? []).map(fromDef)
    options.value = { ...def.options }
    // Preserve pending scopes, but refresh clean metadata affected by DDL
    // (for example an index column renamed or dropped by the column save).
    const refreshIndexes = !columnsOnly || !dirtyIndexes.value
    const refreshForeignKeys = !columnsOnly || !dirtyFKs.value
    if (refreshIndexes) indexes.value = def.indexes ?? []
    if (refreshForeignKeys) foreignKeys.value = def.foreignKeys ?? []
    await nextTick()
    columnBaseline.value=columnsSnapshot()
    if (refreshIndexes) indexBaseline.value=JSON.stringify(indexes.value)
    if (refreshForeignKeys) fkBaseline.value=JSON.stringify(foreignKeys.value)
  } catch (err) {
    notifyError(err)
  } finally {
    if(request===loadRequest) loading.value = false
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
  () => { currentSection.value = 'columns'; void load() },
  { immediate: true },
)

function notifyError(err: unknown): void {
  const message = err instanceof ApiError ? err.message : String(err)
  notification.error({ content: message, duration: 5000 })
}

// --- Row manipulation -----------------------------------------------------

async function addColumn(index = rows.value.length): Promise<void> {
  const key = ++keySeq
  rows.value.splice(index, 0, {
    key,
    name: '',
    originalName: null,
    type: '',
    length: '',
    unsigned: false,
    nullable: true,
    defaultMode: 'none',
    defaultValue: '',
    autoIncrement: false,
    onUpdate: '',
    generated: '',
    generatedExpression: '',
    collation: '',
    comment: '',
    enumValues: [],
    markedForDeletion: false,
  })
  await nextTick()
  columnsPanel.value?.querySelector<HTMLInputElement>(`[data-column-key="${key}"] .struct-field--name input`)?.focus()
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
    if (catalog.schemaAware && r.savedSequence === 'identity') continue
    const enabled = r === row && value
    // PostgreSQL serial is represented by both this flag and a nextval
    // default. Keeping that default would re-enable generation on reload.
    if (r.autoIncrement && !enabled && catalog.schemaAware && r.defaultMode === 'expression' && r.defaultValue.trim().startsWith('nextval(')) {
      r.defaultValue = ''
      r.defaultMode = 'none'
    }
    r.autoIncrement = enabled
  }
}

function isNumeric(row: EditableColumn): boolean {
  return NUMERIC_TYPES.has(row.type.trim().toLowerCase())
}

function isEnumish(row: EditableColumn): boolean {
  return !catalog.schemaAware && ENUM_TYPES.has(row.type.trim().toLowerCase())
}

function isSavedPostgresColumn(row: EditableColumn): boolean {
  return catalog.schemaAware && row.originalName !== null
}

function isSavedPostgresGenerated(row: EditableColumn): boolean {
  return isSavedPostgresColumn(row) && row.generated !== ''
}

function onGeneratedChanged(row: EditableColumn, value: '' | 'VIRTUAL' | 'STORED'): void {
  row.generated = value
  if (value && !catalog.schemaAware) row.nullable = true
}

function defaultSummary(row: EditableColumn): string {
  if (row.generated || row.defaultMode === 'none') return ''
  if (row.defaultMode === 'null') return 'DEFAULT NULL'
  if (row.defaultMode === 'expression') return `DEFAULT ${row.defaultValue}`
  return `DEFAULT '${row.defaultValue.replaceAll("'", "''")}'`
}

// --- Save ----------------------------------------------------------------

const canSave = computed(
  () =>
    tableName.value.trim() !== '' &&
    (!createMode.value || indexesValid.value) &&
    // Create mode needs at least one column (the backend rejects an
    // empty CREATE TABLE anyway); alter mode with zero surviving rows
    // is left to the backend to report.
    (!createMode.value || rows.value.some((r) => !r.markedForDeletion)) &&
    rows.value
      .filter((r) => !r.markedForDeletion)
      .every((r) => r.name.trim() !== '' && r.type.trim() !== '' &&
        (!isEnumish(r) || r.enumValues.length > 0) &&
        (r.generated ? r.generatedExpression.trim() !== '' : r.defaultMode !== 'expression' || r.defaultValue.trim() !== '')),
)

function toColumnDef(row: EditableColumn, after: string): ColumnDef {
  const col: ColumnDef = {
    name: row.name,
    type: row.type,
    nullable: row.nullable,
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
  } else if (row.defaultMode === 'null') {
    col.default = { value: 'NULL', isSqlExpression: true }
  } else if (row.defaultMode !== 'none') {
    col.default = { value: row.defaultValue, isSqlExpression: row.defaultMode === 'expression' }
  }

  if (row.autoIncrement && !row.generated) col.autoIncrement = true
  const onUpdate = row.onUpdate.trim()
  if (onUpdate !== '') col.onUpdate = onUpdate
  if (isEnumish(row)) col.enumValues = [...row.enumValues]
  const collation = row.collation.trim()
  if (collation !== '') col.collation = collation
  if (row.comment !== '') col.comment = row.comment

  // Every column (not only new ones) carries its placement from the
  // desired display order — FIRST for the first row, AFTER the previous
  // non-deleted row otherwise. For an unchanged MODIFY the clause is a
  // harmless, idempotent no-op; for a moved column it is what makes the
  // reorder reach the server at all (buildAlterTable takes the placement
  // from the position field, not from the array order).
  if (!catalog.schemaAware) {
    // The legacy position field reserves FIRST (case-insensitively).
    if (after.toUpperCase() === 'FIRST') col.afterColumn = after
    else col.position = after || 'FIRST'
  }
  return col
}

function buildColumns(): ColumnDef[] {
  const result: ColumnDef[] = []
  let after = ''
  for (const row of rows.value) {
    if (row.markedForDeletion) {
      if (row.originalName) {
        result.push({ name: row.originalName, oldName: row.originalName, type: '', nullable: false })
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
  if(!createMode.value && tableName.value!==props.table && (dirtyIndexes.value || dirtyFKs.value) && !window.confirm(t('navigationWorkflows.discard')))return
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
      await snapshotAll()
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
      await snapshotAll()
      emit('renamed', tableName.value)
      return
    }
    await load(true)
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
      <nav class="structure-sections" :aria-label="t('table.structure')">
        <button v-for="section in sections" :key="section.name" type="button" :data-structure-section="section.name"
          :aria-pressed="section.name === currentSection" @click="currentSection = section.name">
          <UiIcon :name="section.icon" :size="15" />{{ t('structure.' + section.name) }}
        </button>
      </nav>
      <div v-if="createMode || currentSection === 'columns' || currentSection === 'options'" class="structure-editor__header">
        <label class="struct-field struct-field--table-name">
          <span class="struct-field__label">{{ t('structure.tableName') }}</span>
          <UiInput
            v-model:value="tableName"
            :placeholder="t('structure.tableName')"
          />
        </label>
        <UiButton
          type="primary"
          :disabled="!canSave || saving"
          :loading="saving"
          data-testid="struct-save"
          @click="save"
        >
          {{ t(createMode ? 'rowWorkflows.createTable' : 'navigationWorkflows.columnSave') }}
        </UiButton>
      </div>

      <section v-show="currentSection === 'columns'" class="structure-box">
      <header class="structure-columns-heading">
        <h3 class="structure-editor__title">{{ t('structure.columns') }}</h3>
        <UiButton v-if="canPlaceColumns && rows.length" size="small" quaternary data-add-first @click="addColumn(0)">
          {{ t('structure.addFirst') }}
        </UiButton>
      </header>
      <div ref="columnsPanel" class="structure-editor__columns">
        <div v-for="(row, index) in rows" :key="row.key" :data-column-key="row.key" class="structure-column">
          <details
          :open="createMode || !row.originalName"
          class="struct-row"
          :class="{ 'struct-row--deleted': row.markedForDeletion }"
          data-testid="struct-row"
        >
          <summary>
            <UiIcon name="chevron-right" class="struct-disclosure" :size="14" />
            <strong>{{ row.name || '…' }}</strong>
            <code>{{ row.type }}{{ row.length ? `(${row.length})` : '' }}</code>
            <span class="struct-row__flags">{{ row.nullable ? 'NULL' : 'NOT NULL' }}
              <span v-if="row.autoIncrement && !row.generated"> · ID</span>
              <span v-if="!row.generated && row.defaultMode !== 'none'"> · {{ defaultSummary(row) }}</span>
              <span v-if="row.generated"> · {{ row.generated }}</span>
            </span>
          </summary>
          <div class="struct-row__fields">
          <label
            class="struct-field struct-field--name"
            :title="row.originalName ?? ''"
          >
            <span class="struct-field__label">{{ t('structure.name') }}</span>
            <UiInput
              v-model:value="row.name"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label class="struct-field struct-field--type">
            <span class="struct-field__label">{{ t('structure.type') }}</span>
            <UiSelect
              v-model:value="row.type"
              :options="typeOptions"
              :disabled="row.markedForDeletion || isSavedPostgresGenerated(row)"
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
            <UiInput
              v-model:value="row.length"
              :disabled="row.markedForDeletion"
              :readonly="isSavedPostgresGenerated(row)"
            />
          </label>
          <fieldset
            v-if="isEnumish(row)"
            class="struct-field struct-field--enumvalues"
            :disabled="row.markedForDeletion"
          >
            <legend class="struct-field__label">{{ t('structure.enumValues', { type: row.type.toUpperCase() }) }}</legend>
            <div v-for="(_, i) in row.enumValues" :key="i" class="enum-choice-row">
              <textarea v-model="row.enumValues[i]" class="enum-choice" rows="1" :aria-label="t('structure.choiceNumber', { n: i + 1 })" />
              <UiButton size="small" quaternary data-remove-choice :disabled="row.markedForDeletion"
                :aria-label="t('structure.removeChoice', { n: i + 1 })" @click="row.enumValues.splice(i, 1)">×</UiButton>
            </div>
            <UiButton size="small" data-add-choice :disabled="row.markedForDeletion" @click="row.enumValues.push('')">
              {{ t('structure.addChoice') }}
            </UiButton>
          </fieldset>
          <label
            v-if="!row.generated"
            class="struct-field struct-field--defaultmode"
          >
            <span class="struct-field__label">{{ t('structure.defaultValue') }}</span>
            <UiSelect v-model:value="row.defaultMode" :options="defaultOptions" class="default-mode"
              :aria-label="t('structure.defaultValue')" :disabled="row.markedForDeletion" />
          </label>
          <label
            v-if="!row.generated && (row.defaultMode === 'literal' || row.defaultMode === 'expression')"
            class="struct-field struct-field--default"
          >
            <span class="struct-field__label">{{ row.defaultMode === 'literal' ? t('structure.literalDefault') : t('structure.sqlExpression') }}</span>
            <UiInput
              v-model:value="row.defaultValue"
              type="textarea"
              :autosize="{ minRows: 1, maxRows: 4 }"
              :placeholder="row.defaultMode === 'literal' ? t('structure.emptyString') : t('structure.sqlExpression')"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label
            v-if="row.generated"
            class="struct-field struct-field--genexpr"
          >
            <span class="struct-field__label">{{ t('structure.generatedExpression') }}</span>
            <UiInput
              v-model:value="row.generatedExpression"
              :disabled="row.markedForDeletion"
              :readonly="isSavedPostgresGenerated(row)"
              :placeholder="t('structure.generatedHint')"
            />
          </label>
          <label v-if="!catalog.schemaAware || row.onUpdate" class="struct-field struct-field--onupdate">
            <span class="struct-field__label">{{ t('structure.onUpdate') }}</span>
            <UiInput
              v-model:value="row.onUpdate"
              :readonly="catalog.schemaAware"
              :disabled="row.markedForDeletion"
              :placeholder="t('structure.onUpdateHint')"
            />
          </label>
          <label class="struct-field struct-field--generated">
            <span class="struct-field__label">{{ t('structure.generated') }}</span>
            <UiSelect
              :value="row.generated"
              :options="generatedOptions"
              :disabled="row.markedForDeletion || isSavedPostgresColumn(row)"
              @update:value="onGeneratedChanged(row, $event as EditableColumn['generated'])"
            />
            <small v-if="isSavedPostgresGenerated(row)" class="struct-field__help">{{ t('structure.savedGenerationHint') }}</small>
          </label>
          <label v-if="!catalog.schemaAware || row.collation" class="struct-field struct-field--collation">
            <span class="struct-field__label">{{ t('structure.collation') }}</span>
            <UiInput
              v-model:value="row.collation"
              :readonly="catalog.schemaAware"
              :disabled="row.markedForDeletion"
            />
          </label>
          <label class="struct-field struct-field--comment">
            <span class="struct-field__label">{{ t('structure.comment') }}</span>
            <UiInput
              v-model:value="row.comment"
              :disabled="row.markedForDeletion"
            />
          </label>
          <div class="struct-row__footer">
            <div class="struct-row__checks">
              <div class="struct-field--nullable">
                <UiCheckbox
                  :label="t('structure.nullable')"
                  :checked="row.nullable"
                  :disabled="(row.generated !== '' && !catalog.schemaAware) || row.markedForDeletion"
                  @update:checked="row.nullable = $event"
                />
              </div>
              <div v-if="!catalog.schemaAware && isNumeric(row)" class="struct-field--unsigned">
                <UiCheckbox
                  :label="t('structure.unsigned')"
                  :checked="row.unsigned"
                  :disabled="row.markedForDeletion"
                  @update:checked="row.unsigned = $event"
                />
              </div>
              <div v-if="!catalog.schemaAware || row.savedSequence || /^(int|integer|bigint)$/i.test(row.type)" class="struct-field--autoinc">
                <UiCheckbox
                  :label="t(catalog.schemaAware ? 'structure.sequence' : 'structure.autoIncrement')"
                  :checked="row.autoIncrement"
                  :disabled="row.generated !== '' || row.markedForDeletion || (isSavedPostgresColumn(row) && row.savedSequence !== 'serial')"
                  @update:checked="setAutoIncrement(row, $event)"
                />
              </div>
            </div>
            <div class="struct-row__buttons">
              <UiButton
                size="small"
                quaternary
                :disabled="index === 0"
                v-if="canPlaceColumns"
                data-testid="col-up"
                :title="t('structure.moveUp')"
                @click="moveRow(index, -1)"
              >
                ↑
              </UiButton>
              <UiButton
                size="small"
                quaternary
                :disabled="index === rows.length - 1"
                v-if="canPlaceColumns"
                data-testid="col-down"
                :title="t('structure.moveDown')"
                @click="moveRow(index, 1)"
              >
                ↓
              </UiButton>
              <UiButton
                size="small"
                quaternary
                type="error"
                data-testid="col-delete"
                :title="row.markedForDeletion ? t('structure.restore') : t('structure.delete')"
                @click="toggleDeletion(row)"
              >
                ✕
              </UiButton>
            </div>
          </div>
          <div
            v-if="row.generated && !isSavedPostgresColumn(row)"
            class="struct-row__hint"
          >
            {{ t('structure.generatedHint') }}
          </div>
          </div>
          </details>
          <UiButton v-if="canPlaceColumns && !row.markedForDeletion" class="insert-after" size="small" quaternary
            data-add-after :title="t('structure.addAfter', { name: row.name || '…' })"
            :aria-label="t('structure.addAfter', { name: row.name || '…' })"
            @click="addColumn(index + 1)">＋</UiButton>
        </div>
        <UiButton
          size="small"
          data-testid="struct-add"
          @click="addColumn()"
        >
          {{ t('structure.addColumn') }}
        </UiButton>
      </div>
      <p v-if="!canPlaceColumns" class="struct-row__hint">{{ t('structure.appendOnly') }}</p>

      </section>
      <section v-show="currentSection === 'indexes'" class="structure-box">
      <!-- The indexes endpoint only exists for a saved table; on create
           the inline indexes list of the PUT body is used instead. -->
      <IndexEditor
        :db="db"
        :table="table"
        v-model:indexes="indexes"
        :draft="createMode"
        @validity="indexesValid = $event"
        :column-names="rows.filter((r) => !r.markedForDeletion).map((r) => r.name)"
        @saved="indexesSaved"
      />

      </section>
      <section v-show="currentSection === 'options'" class="structure-box">
      <OptionsEditor
        v-model:options="options"
      />

      </section>
      <section v-show="currentSection === 'foreignKeys'" class="structure-box">
      <ForeignKeyEditor v-model="foreignKeys" :db="db" :table="table"
        :column-names="rows.filter(r => !r.markedForDeletion).map(r => r.name)" @saved="fksSaved" />
      </section>

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
  flex-wrap:wrap;
  justify-content:space-between;
  padding: 2px 0 10px;
  gap: 12px;
}

.structure-editor__header .struct-field--table-name {
  display:grid; gap:4px; flex:1; min-width:min(100%, 180px);
  max-width: 320px;
}

.structure-editor__title {
  margin: 12px 0 0;
  font-size: 14px;
}

.structure-sections { display:flex; align-items:center; flex-wrap:wrap; gap:3px; width:fit-content; margin-bottom:8px; padding:3px; background:var(--db-sidebar); border:1px solid var(--db-line); border-radius:9px; }
.structure-sections button { display:inline-flex; align-items:center; gap:6px; padding:6px 10px; background:none; border:1px solid transparent; border-radius:6px; font:inherit; font-size:12px; color:var(--db-muted); cursor:pointer; }
.structure-sections button[aria-pressed=true] { background:var(--db-panel); color:var(--db-accent); border-color:var(--db-line); }
.structure-box { min-width:0; padding:16px; border:1px solid var(--db-line); border-radius:10px; background:var(--db-panel); }
.struct-field__label { font-size:12px; font-weight:550; color:var(--db-muted); }
.structure-editor__columns {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}
.structure-columns-heading { display: flex; align-items: center; justify-content:space-between; gap: 12px; margin:0 0 12px; }
.structure-columns-heading h3 { margin: 0; }
.structure-column { width: 100%; position: relative; }
.structure-column:has(.insert-after) summary { padding-inline-end: 40px; }
.insert-after { position: absolute; inset-inline-end: 8px; top: 6px; }

.struct-row { width:100%; border:1px solid var(--db-line); border-radius:7px; box-sizing:border-box; background:var(--db-panel); }
.struct-row summary { display:grid; grid-template-columns:15px minmax(0,1fr) minmax(70px,.8fr) minmax(0,1.4fr); gap:8px; align-items:center; min-height:40px; padding:6px 12px; list-style:none; cursor:pointer; font-size:12px; overflow-wrap:anywhere; }
.struct-row summary::-webkit-details-marker { display:none; }
.struct-row summary:hover { background:var(--db-sidebar); }
.struct-row summary strong, .struct-row summary code { font-family:var(--db-mono); }
.struct-row summary code { color:var(--db-accent); }
.struct-row__flags, .struct-disclosure { color:var(--db-muted); }
.struct-row[open] .struct-disclosure { transform:rotate(90deg); }
@media (max-width:620px) {
  .structure-box { padding:10px; }
  .structure-sections button { padding:6px 8px; }
  .struct-row summary { grid-template-columns:12px minmax(0,1fr) minmax(70px,.9fr); }
  .struct-row__flags { grid-column:2 / -1; font-size:11px; }
}
.enum-choice {
  box-sizing: border-box;
  padding: 6px 8px;
  border: 1px solid var(--db-line);
  border-radius: 6px;
  color: inherit;
  background: var(--db-panel);
  font: inherit;
}
.struct-field--enumvalues {
  display: grid;
  gap: 4px;
  margin: 0;
  padding: 8px;
  border: 1px solid #8884;
  border-radius: 4px;
}
.enum-choice-row { display: flex; align-items: center; gap: 4px; }
.enum-choice { resize: vertical; width: 100%; min-width: 0; }
.struct-field--enumvalues > button { justify-self: start; }
.struct-row__fields {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: start;
  gap: 12px 16px;
  margin-top: 6px;
  padding: 12px;
  border-top: 1px solid var(--db-line);
  width: 100%;
}

.struct-row[open] { container-type: inline-size; }

@container (min-width: 32rem) {
  .struct-row__fields { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@container (min-width: 53rem) {
  .struct-row__fields { grid-template-columns: repeat(4, minmax(0, 1fr)); }
  .struct-field--enumvalues { grid-column: 3; grid-row: 1 / span 3; }
}

.struct-row__fields > label {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.struct-row--deleted .struct-field__label,
.struct-row--deleted :deep(.ui-input__control) {
  text-decoration: line-through;
  opacity: 0.5;
}

.struct-row .struct-field {
  min-width: 0;
}

.struct-field__help { display: block; max-width: 32ch; margin-top: 4px; }

.struct-row__footer {
  grid-column: 1 / -1;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 16px;
  padding-top: 8px;
  border-top: 1px solid var(--db-line);
}

.struct-row__checks {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 20px;
}

.struct-row__checks :deep(.ui-checkbox) {
  display: flex;
  align-items: center;
  min-height: 32px;
}

.struct-row__buttons {
  display: flex;
  margin-inline-start: auto;
  gap: 4px;
}

.struct-row__hint {
  grid-column: 1 / -1;
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

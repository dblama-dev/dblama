<template>
  <n-modal
    :show="show"
    preset="card"
    :title="mode === 'insert' ? t('row.insert') : t('row.edit')"
    class="row-modal"
    :style="{ width: '560px' }"
    @update:show="emit('update:show', $event)"
  >
    <div class="row-modal__fields">
      <div v-for="col in columns" :key="col.name" class="row-modal__field">
        <span class="row-modal__label">{{ col.name }}</span>
        <div class="row-modal__control">
          <n-input-number
            v-if="isNumeric(col.type)"
            :value="numericValue(col.name)"
            :disabled="isNulled(col)"
            size="small"
            class="row-modal__input"
            @update:value="(v: number | null) => setValue(col.name, v)"
          />
          <n-select
            v-else-if="enumOptions(col).length > 0"
            :value="stringValue(col.name)"
            :options="enumOptions(col)"
            :disabled="isNulled(col)"
            size="small"
            class="row-modal__input"
            @update:value="(v: string | null) => setValue(col.name, v ?? '')"
          />
          <n-input
            v-else
            :value="stringValue(col.name)"
            :type="isLongText(col.type) ? 'textarea' : 'text'"
            :disabled="isNulled(col)"
            size="small"
            class="row-modal__input"
            @update:value="(v: string) => setValue(col.name, v)"
          />
        </div>
        <n-checkbox
          v-if="col.nullable"
          class="row-modal__null"
          size="small"
          :checked="nulls[col.name] === true"
          @update:checked="(v: boolean) => setNulled(col, v)"
        >
          {{ t('row.nullToggle') }}
        </n-checkbox>
        <span v-else class="row-modal__required">{{ t('row.required') }}</span>
      </div>
    </div>
    <template #footer>
      <div class="row-modal__footer">
        <n-button size="small" @click="close">{{ t('row.cancel') }}</n-button>
        <n-button
          size="small"
          type="primary"
          :loading="saving"
          :disabled="!canSave"
          @click="save"
        >
          {{ t('row.save') }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotification } from 'naive-ui'
import { NButton, NCheckbox, NInput, NInputNumber, NModal, NSelect } from 'naive-ui'

import { api, ApiError } from '../api/client'
import type { ColumnDef } from '../api/types'

// One field per column: numeric types -> n-input-number, enums -> n-select,
// everything else -> n-input (textarea for long text). Each nullable column
// gets a NULL toggle: checked means "send null" and disables the input.

const props = defineProps<{
  show: boolean
  mode: 'insert' | 'edit'
  rowsPath: string
  columns: ColumnDef[]
  /** Original values of the edited row (edit mode only). */
  row: Record<string, unknown> | null
  /** Exact-match key of the edited row for the PATCH where clause. */
  rowPk: Record<string, unknown> | null
}>()

const emit = defineEmits<{
  'update:show': [show: boolean]
  saved: []
}>()

const { t } = useI18n()
const notification = useNotification()

// --- Input type helpers -------------------------------------------------

const NUMERIC_PREFIXES = [
  'int',
  'bigint',
  'smallint',
  'tinyint',
  'mediumint',
  'decimal',
  'numeric',
  'float',
  'double',
]

const LONG_TEXT_RE = /^(text|longtext|mediumtext|tinytext|blob|longblob|mediumblob|tinyblob|json)/

function isNumeric(type: string): boolean {
  return NUMERIC_PREFIXES.some((prefix) => type.startsWith(prefix))
}

function isLongText(type: string): boolean {
  return LONG_TEXT_RE.test(type)
}

function enumOptions(col: ColumnDef): { label: string; value: string }[] {
  return (col.enumValues ?? []).map((v) => ({ label: v, value: v }))
}

// --- Form state ---------------------------------------------------------

// values holds the raw control values: string | number | null. nulls holds
// the per-column NULL toggles; a checked toggle wins over any value.
const values = reactive<Record<string, string | number | null>>({})
const nulls = reactive<Record<string, boolean>>({})
const saving = ref(false)

function stringValue(name: string): string {
  const v = values[name]
  return typeof v === 'number' ? String(v) : (v ?? '')
}

function numericValue(name: string): number | null {
  const v = values[name]
  if (v === null || v === undefined || v === '') return null
  const n = Number(v)
  return Number.isNaN(n) ? null : n
}

function setValue(name: string, v: string | number | null): void {
  values[name] = v
}

function isNulled(col: ColumnDef): boolean {
  return nulls[col.name] === true
}

function setNulled(col: ColumnDef, checked: boolean): void {
  nulls[col.name] = checked
}

// Snapshot of the values the form was prefilled with (post coercion, with
// a checked NULL toggle recorded as null). The edit diff runs against this
// snapshot, NOT against props.row: the backend serializes DECIMAL (and
// other string-serialized numerics) as strings while the form coerces them
// to numbers, and comparing coerced-vs-raw would flag every untouched
// decimal as changed — writing a precision-lost Number back on a plain save.
const prefill: Record<string, string | number | null> = {}

// PINNED: the form is initialized exactly once per modal OPEN (plus
// immediate init for an initially-open mount). The TableDef fetch lands
// AFTER the modal opens and only enriches the columns prop — re-running
// initForm on that arrival would wipe whatever the user already typed, so
// a columns change mid-open deliberately does NOT reset the form (fields
// are keyed by column name, which the enrichment preserves).
watch(
  () => [props.show, props.mode, props.row] as const,
  ([show]) => {
    if (show) initForm()
  },
  { immediate: true },
)

function initForm(): void {
  for (const key of Object.keys(values)) delete values[key]
  for (const key of Object.keys(nulls)) delete nulls[key]
  for (const key of Object.keys(prefill)) delete prefill[key]
  for (const col of props.columns) {
    if (props.mode === 'edit' && props.row !== null) {
      initFromRow(col)
    } else {
      initFromDefault(col)
    }
    prefill[col.name] = nulls[col.name] === true ? null : (values[col.name] ?? null)
  }
}

function initFromRow(col: ColumnDef): void {
  const orig = props.row?.[col.name]
  if (orig === null || orig === undefined) {
    nulls[col.name] = col.nullable
    values[col.name] = ''
    return
  }
  nulls[col.name] = false
  if (isNumeric(col.type)) {
    const n = Number(orig)
    values[col.name] = Number.isNaN(n) ? null : n
  } else if (typeof orig === 'string') {
    values[col.name] = orig
  } else {
    values[col.name] = String(orig)
  }
}

function initFromDefault(col: ColumnDef): void {
  nulls[col.name] = false
  const def = col.default
  if (def && !def.isSqlExpression && def.value !== '') {
    if (isNumeric(col.type)) {
      const n = Number(def.value)
      values[col.name] = Number.isNaN(n) ? null : n
    } else {
      values[col.name] = def.value
    }
    return
  }
  // No usable default: empty control, the user can pick NULL instead.
  values[col.name] = isNumeric(col.type) ? null : ''
}

// --- Building the payload ----------------------------------------------

// PINNED RULE (insert): the RowValues map contains EVERY column of the
// table — null when the NULL toggle is checked, the numeric control value
// (null when empty) for numeric types, and the raw string (possibly '')
// for everything else.
function buildValues(): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const col of props.columns) {
    if (nulls[col.name] === true) {
      out[col.name] = null
    } else if (isNumeric(col.type)) {
      out[col.name] = numericValue(col.name)
    } else {
      out[col.name] = stringValue(col.name)
    }
  }
  return out
}

// PINNED RULE (edit): only the fields whose built value differs from the
// prefilled snapshot are sent. Comparing coerced-vs-coerced keeps untouched
// string-serialized numerics (DECIMAL, BIGINT UNSIGNED > 2^63, ...) out of
// the payload; a NULL toggle state counts as part of the snapshot.
function buildChangedValues(): Record<string, unknown> {
  const built = buildValues()
  const changed: Record<string, unknown> = {}
  for (const [name, value] of Object.entries(built)) {
    const orig = nulls[name] === true ? null : (prefill[name] ?? null)
    if (value !== orig) {
      changed[name] = value
    }
  }
  return changed
}

const canSave = computed(() => {
  for (const col of props.columns) {
    // An AUTO_INCREMENT column fills itself: an untouched auto-inc
    // column must not block the save (buildValues sends null for it).
    if (col.nullable || col.autoIncrement) continue
    if (nulls[col.name] === true) return false
    const v = values[col.name]
    if (v === null || v === undefined || v === '') return false
  }
  return props.columns.length > 0
})

// --- Save / close -------------------------------------------------------

function close(): void {
  emit('update:show', false)
}

async function save(): Promise<void> {
  if (!canSave.value || saving.value) return
  saving.value = true
  try {
    if (props.mode === 'insert') {
      await api<{ lastInsertId: number }>('POST', props.rowsPath, buildValues())
    } else {
      const changed = buildChangedValues()
      if (Object.keys(changed).length === 0) {
        // Nothing changed: close without a request and without a refetch.
        close()
        return
      }
      await api('PATCH', props.rowsPath, { where: props.rowPk, values: changed })
    }
  } catch (err) {
    const message = err instanceof ApiError ? err.message : String(err)
    notification.error({ content: message, duration: 5000 })
    return
  } finally {
    saving.value = false
  }
  emit('saved')
  close()
}
</script>

<style scoped>
.row-modal__fields {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.row-modal__field {
  display: grid;
  grid-template-columns: 140px 1fr auto;
  align-items: center;
  gap: 8px;
}

.row-modal__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-modal__input {
  width: 100%;
}

.row-modal__required {
  font-size: 12px;
  opacity: 0.6;
}

.row-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>

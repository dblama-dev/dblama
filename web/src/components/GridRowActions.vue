<template>
  <div class="row-actions" role="group" :aria-label="t('grid.actions')">
    <n-button v-for="action in actions" :key="action" quaternary size="small"
      :title="t('grid.' + action)" :aria-label="t('grid.' + action)" :data-testid="testIDs[action]"
      :disabled="action === 'preview' && !canPreview" @click="emit('action', action)">
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path :d="paths[action]"/></svg>
    </n-button>
    <n-popconfirm :show="pending !== null" @update:show="toggleDelete" @positive-click="confirmDelete">
      <template #trigger>
        <n-button quaternary size="small" type="error" :disabled="!target" :title="t('grid.delete')" :aria-label="t('grid.delete')">
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/></svg>
        </n-button>
      </template>
      {{ t('grid.confirmDelete') }} {{ tableName }} {{ JSON.stringify(pending) }}
    </n-popconfirm>
  </div>
</template>

<script setup lang="ts">
import { shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NPopconfirm } from 'naive-ui'

const actions = ['edit', 'preview', 'copy'] as const
type Action = typeof actions[number]
const props = defineProps<{ target: Record<string, unknown> | null; tableName: string; canPreview: boolean }>()
const emit = defineEmits<{ action: [Action]; delete: [Record<string, unknown>] }>()
const { t } = useI18n()
const pending = shallowRef<Record<string, unknown> | null>(null)
const paths = {
  edit: 'M4 20l4-1 12-12a2.1 2.1 0 0 0-3-3L5 16zM14.5 6.5l3 3',
  preview: 'M2 12s3.7-7 10-7 10 7 10 7-3.7 7-10 7-10-7-10-7ZM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z',
  copy: 'M8 8h12v13H8zM15 8V3H3v13h5',
}
const testIDs = { edit: 'edit-row', preview: 'row-inspect', copy: 'copy-row' }

function toggleDelete(open: boolean) {
  pending.value = open && props.target ? { ...props.target } : null
}
function confirmDelete() {
  if (pending.value) emit('delete', pending.value)
  pending.value = null
}
// A keyless table can reuse a displayed row position after filtering or polling.
watch([() => props.target, () => props.tableName], () => { pending.value = null })
</script>

<style scoped>
.row-actions { display: inline-flex; align-items: center; gap: 2px; white-space: nowrap; }
.row-actions .n-button { width: 30px; height: 32px; padding: 0; }
.row-actions svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.75; stroke-linecap: round; stroke-linejoin: round; }
@media (pointer: coarse) { .row-actions .n-button { height: 38px; } }
</style>

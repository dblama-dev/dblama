<template>
  <div class="row-actions" role="group" :aria-label="t('grid.actions')">
    <template v-if="compact">
      <ui-dropdown :options="mobileOptions" @select="selectMobile">
        <button class="row-action" type="button" :title="t('grid.actions')" :aria-label="t('grid.actions')"><UiIcon name="more" /></button>
      </ui-dropdown>
      <ui-modal v-if="pending" :show="true" preset="dialog" type="warning" :title="t('grid.delete')"
        :positive-text="t('grid.delete')" :negative-text="t('row.cancel')" @positive-click="confirmDelete"
        @negative-click="pending = null" @update:show="pending = null">
        {{ t('grid.confirmDelete') }} {{ tableName }} {{ JSON.stringify(pending) }}
      </ui-modal>
    </template>
    <template v-else>
    <button class="row-action" type="button" v-for="action in actions" :class="{ 'secondary-action': action !== 'edit' }" :key="action"
      :title="t('grid.' + action)" :aria-label="t('grid.' + action)" :data-testid="testIDs[action]"
      :disabled="action === 'preview' && !canPreview" @click="emit('action', action)">
      <UiIcon :name="action === 'preview' ? 'eye' : action" />
    </button>
    <ui-popconfirm :show="pending !== null" @update:show="toggleDelete" @positive-click="confirmDelete">
      <template #trigger>
        <button class="row-action secondary-action row-action--delete" type="button" :disabled="!target" :title="t('grid.delete')" :aria-label="t('grid.delete')">
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/></svg>
        </button>
      </template>
      {{ t('grid.confirmDelete') }} {{ tableName }} {{ JSON.stringify(pending) }}
    </ui-popconfirm>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiDropdown, UiModal, UiPopconfirm } from '../ui'
import UiIcon from './UiIcon.vue'

const actions = ['edit', 'preview', 'copy'] as const
type Action = typeof actions[number]
const props = defineProps<{ target: Record<string, unknown> | null; tableName: string; canPreview: boolean; compact: boolean }>()
const emit = defineEmits<{ action: [Action]; delete: [Record<string, unknown>] }>()
const { t } = useI18n()
const pending = shallowRef<Record<string, unknown> | null>(null)
const mobileOptions = computed(() => ([...actions, 'delete'] as const).map(action => ({
  label: t('grid.' + action), key: action,
  disabled: (action === 'preview' && !props.canPreview) || (action === 'delete' && !props.target),
})))
function selectMobile(action: Action | 'delete'): void {
  if (action === 'delete') toggleDelete(true)
  else emit('action', action)
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
.row-action { display:inline-grid; place-items:center; width:30px; height:32px; padding:0; border:0; border-radius:5px; color:var(--db-muted); background:none; cursor:pointer; }
.row-action:hover,.row-action:focus-visible { color:var(--db-accent); background:var(--db-tint); }
.row-action--delete:hover,.row-action--delete:focus-visible { color:var(--db-danger,#c54754); }
.row-action:disabled { cursor:default; opacity:.45; }
.row-actions svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.75; stroke-linecap: round; stroke-linejoin: round; }
@media (pointer: coarse) { .row-action { height:38px; } }
</style>

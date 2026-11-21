<script setup lang="ts">
import { computed } from 'vue'
import UiIcon from '../../components/UiIcon.vue'
import { useCollectionLabels } from './messages'

const props = withDefaults(defineProps<{
  page: number
  pageSize: number
  itemCount: number
  pageSizes?: number[]
  showSizePicker?: boolean
  pageSlot?: number
}>(), { pageSlot: 7 })
const emit = defineEmits<{ 'update:page': [page: number]; 'update:page-size': [size: number] }>()
const { t } = useCollectionLabels()
const lastPage = computed(() => Math.max(1, Math.ceil(props.itemCount / props.pageSize)))
const current = computed(() => Math.min(Math.max(1, props.page), lastPage.value))
const pages = computed<(number | 'gap-start' | 'gap-end')[]>(() => {
  const slots = Math.max(3, props.pageSlot)
  const last = lastPage.value
  if (last <= slots) return Array.from({ length: last }, (_, i) => i + 1)

  // Compact pagers keep the current page between the two arrow controls.
  // Wider pagers also expose both endpoints and mark skipped ranges.
  if (slots < 5) {
    const first = Math.min(Math.max(1, current.value - 1), last - slots + 1)
    return Array.from({ length: slots }, (_, i) => first + i)
  }
  const interior = slots - 4
  const start = Math.min(Math.max(3, current.value - Math.floor(interior / 2)), last - interior - 1)
  const end = start + interior - 1
  if (start === 3) return [...Array.from({ length: slots - 2 }, (_, i) => i + 1), 'gap-end', last]
  if (end === last - 2) return [1, 'gap-start', ...Array.from({ length: slots - 2 }, (_, i) => last - slots + 3 + i)]
  return [1, 'gap-start', ...Array.from({ length: interior }, (_, i) => start + i), 'gap-end', last]
})
</script>

<template>
  <nav class="ui-pagination" :aria-label="t('pagination')">
    <button type="button" :aria-label="t('previous')" :disabled="current <= 1" @click="emit('update:page', current - 1)">
      <UiIcon name="chevron-left" :size="14" />
    </button>
    <template v-for="item in pages" :key="item">
      <button v-if="typeof item === 'number'" type="button" class="ui-pagination-item"
        :class="{ 'ui-pagination-item--active': current === item }" :aria-label="t('page', { number: item })"
        :aria-current="current === item ? 'page' : undefined" @click="item !== current && emit('update:page', item)">
        {{ item }}
      </button>
      <span v-else class="ui-pagination-gap" aria-hidden="true">…</span>
    </template>
    <button type="button" :aria-label="t('next')" :disabled="current >= lastPage" @click="emit('update:page', current + 1)">
      <UiIcon name="chevron-right" :size="14" />
    </button>
    <select v-if="showSizePicker" :aria-label="t('rowsPerPage')" :value="pageSize"
      @change="emit('update:page-size', Number(($event.target as HTMLSelectElement).value))">
      <option v-for="size in pageSizes" :key="size" :value="size">{{ t('size', { size }) }}</option>
    </select>
  </nav>
</template>

<style scoped>
.ui-pagination { display: flex; align-items: center; flex-wrap: wrap; gap: 3px; color: var(--db-ink); font-size: 12px; font-variant-numeric: tabular-nums; }
button, select { color: inherit; font: inherit; background: var(--db-panel); border: 1px solid var(--db-line); border-radius: 6px; min-height: 29px; }
button { display: grid; place-items: center; min-width: 28px; padding: 2px 5px; cursor: pointer; }
button:disabled { opacity: .46; cursor: default; }
button:enabled:hover { border-color: var(--db-accent); color: var(--db-accent); }
button[aria-current=page] { background: var(--db-tint); color: var(--db-accent); border-color: var(--db-accent); font-weight: 600; }
.ui-pagination-gap { text-align: center; min-width: 15px; color: var(--db-muted); }
select { margin-inline-start: 3px; padding-inline: 6px; }
</style>

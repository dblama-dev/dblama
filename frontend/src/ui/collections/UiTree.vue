<script setup lang="ts">
import { computed, ref, type VNodeChild } from 'vue'
import UiIcon from '../../components/UiIcon.vue'
import { useCollectionLabels } from './messages'
import type { TreeOption } from './types'

interface VisibleNode { node: TreeOption; level: number; parent?: string }
const props = defineProps<{
  data: TreeOption[]
  expandedKeys: string[]
  selectedKeys: string[]
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:expanded-keys': [keys: string[]]; 'update:selected-keys': [keys: string[]] }>()
const { t } = useCollectionLabels()
const tree = ref<HTMLElement>()
const activeKey = ref('')
const RenderContent = (props: { content: VNodeChild }) => props.content
const expanded = computed(() => new Set(props.expandedKeys))
const visible = computed(() => {
  const items: VisibleNode[] = []
  function append(nodes: TreeOption[], level: number, parent?: string) {
    for (const node of nodes) {
      items.push({ node, level, parent })
      if (node.children && expanded.value.has(node.key)) append(node.children, level + 1, node.key)
    }
  }
  append(props.data, 1)
  return items
})
const tabKey = computed(() => visible.value.find(({ node }) => node.key === activeKey.value)?.node.key
  ?? visible.value.find(({ node }) => props.selectedKeys.includes(node.key))?.node.key
  ?? visible.value[0]?.node.key)

function activate(node: TreeOption): void {
  if (props.disabled) return
  activeKey.value = node.key
  if (node.children) {
    const keys = new Set(expanded.value)
    if (keys.has(node.key)) keys.delete(node.key)
    else keys.add(node.key)
    emit('update:expanded-keys', [...keys])
  } else {
    emit('update:selected-keys', [node.key])
  }
}

function focusItem(index: number): void {
  const item = tree.value?.querySelectorAll<HTMLElement>('[role=treeitem]')[index]
  item?.focus()
}

function keydown(event: KeyboardEvent, item: VisibleNode, index: number): void {
  if (props.disabled || event.target !== event.currentTarget) return
  const { node } = item
  switch (event.key) {
    case 'ArrowDown': focusItem(Math.min(index + 1, visible.value.length - 1)); break
    case 'ArrowUp': focusItem(Math.max(index - 1, 0)); break
    case 'Home': focusItem(0); break
    case 'End': focusItem(visible.value.length - 1); break
    case 'ArrowRight':
      if (node.children && !expanded.value.has(node.key)) activate(node)
      else if (node.children?.length) focusItem(index + 1)
      break
    case 'ArrowLeft':
      if (node.children && expanded.value.has(node.key)) activate(node)
      else if (item.parent) focusItem(visible.value.findIndex(entry => entry.node.key === item.parent))
      break
    case ' ':
    case 'Enter': activate(node); break
    default: return
  }
  event.preventDefault()
  event.stopPropagation()
}
</script>

<template>
  <div ref="tree" class="ui-tree" role="tree" :aria-label="t('objects')" :aria-busy="!!disabled">
    <div v-for="(item, index) in visible" :key="item.node.key" class="ui-tree-node" role="treeitem"
      :aria-label="item.node.label" :aria-level="item.level" :aria-disabled="disabled || undefined"
      :aria-expanded="item.node.children ? expanded.has(item.node.key) : undefined"
      :aria-selected="item.node.children ? undefined : selectedKeys.includes(item.node.key)"
      :tabindex="!disabled && tabKey === item.node.key ? 0 : -1" :style="{ paddingInlineStart: 5 + (item.level - 1) * 15 + 'px' }"
      @focus="activeKey = item.node.key" @keydown="keydown($event, item, index)" @click="activate(item.node)">
      <span v-if="item.node.children" class="ui-tree-disclosure">
        <UiIcon :name="expanded.has(item.node.key) ? 'chevron-down' : 'chevron-right'" :size="13" />
      </span>
      <div class="ui-tree-node-content">
        <span v-if="item.node.prefix" class="ui-tree-node-content__prefix"><RenderContent :content="item.node.prefix()" /></span>
        <span class="ui-tree-node-content__text" :title="item.node.label">{{ item.node.label }}</span>
      </div>
      <span v-if="item.node.suffix" class="ui-tree-suffix" :inert="disabled || undefined" @click.stop>
        <RenderContent :content="item.node.suffix()" />
      </span>
    </div>
    <p v-if="!visible.length" class="ui-tree-empty">{{ t(disabled ? 'loading' : 'empty') }}</p>
  </div>
</template>

<style scoped>
.ui-tree { min-width: 0; color: var(--db-ink); font-size: 12px; }
.ui-tree-node { display: flex; align-items: center; gap: 4px; min-width: 0; min-height: 29px; padding-inline-end: 3px; border-radius: 5px; cursor: pointer; }
.ui-tree-node[aria-expanded] { font-weight: 550; }
.ui-tree-node[aria-disabled=true] { opacity: .55; cursor: progress; }
.ui-tree-node:not([aria-disabled=true]):hover { background: var(--db-tint); }
.ui-tree-node[aria-selected=true] { color: var(--db-accent); background: var(--db-tint); }
.ui-tree-node[aria-selected=true] .ui-tree-node-content__text { font-weight: 600; }
.ui-tree-node:focus-visible { outline-offset: -2px; }
.ui-tree-node-content { flex: 1; display: flex; align-items: center; min-width: 0; gap: 6px; }
.ui-tree-node-content__text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ui-tree-disclosure, .ui-tree-node-content__prefix { display: inline-flex; flex: none; align-items: center; color: var(--db-muted); }
.ui-tree-suffix { display: inline-flex; align-items: center; flex: none; }
.ui-tree-empty { padding: 8px; margin: 0; color: var(--db-muted); }
</style>

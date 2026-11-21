<template>
  <Trigger />
  <Teleport v-if="show" :to="host">
    <div ref="panel" :id="id" :aria-labelledby="role === 'menu' ? anchor?.id : undefined"
      v-bind="$attrs" class="ui-floating" :role="role" :style="position" tabindex="-1" @keydown="panelKeys">
      <slot />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { cloneVNode, Comment, nextTick, onBeforeUnmount, ref, shallowRef, useId, watch, type CSSProperties, type Slot } from 'vue'
import { keepTabInside } from './dialogFocus'

defineOptions({ inheritAttrs: false })
const props = defineProps<{ show: boolean; role: 'menu' | 'alertdialog'; triggerSlot?: Slot }>()
const emit = defineEmits<{ 'update:show': [show: boolean] }>()
const id = useId()
const triggerId = useId()
const host = shallowRef<HTMLElement | string>('body')
const panel = ref<HTMLElement>()
const position = shallowRef<CSSProperties>({ visibility: 'hidden' })
const anchor = shallowRef<HTMLElement | null>(null)
let focusLast = false
let restoreFocus = true

function setOpen(value: boolean, restore = true) {
  restoreFocus = restore
  emit('update:show', value)
}

function openFrom(event: Event, last = false) {
  anchor.value = event.currentTarget as HTMLElement
  if (anchor.value.matches(':disabled,[aria-disabled=true]')) return
  host.value = anchor.value.closest('dialog') ?? 'body'
  focusLast = last
  setOpen(!props.show)
}

function Trigger() {
  const trigger = props.triggerSlot?.().find(node => node.type !== Comment)
  if (!trigger) return
  return cloneVNode(trigger, {
    id: trigger.props?.id ?? triggerId,
    'aria-haspopup': props.role === 'menu' ? 'menu' : 'dialog',
    'aria-expanded': props.show,
    'aria-controls': props.show ? id : undefined,
    onClick: (event: MouseEvent) => openFrom(event),
    onKeydown: (event: KeyboardEvent) => {
      if (props.role !== 'menu' || !['ArrowDown', 'ArrowUp'].includes(event.key)) return
      event.preventDefault()
      openFrom(event, event.key === 'ArrowUp')
    },
  })
}

function locate() {
  if (!panel.value || !anchor.value) return
  const rect = anchor.value.getBoundingClientRect()
  const width = panel.value.offsetWidth
  const height = panel.value.offsetHeight
  const gap = 6, margin = 8
  const below = window.innerHeight - rect.bottom - gap - margin
  const above = rect.top - gap - margin
  const useTop = below < height && above > below
  const top = useTop ? rect.top - gap - height : rect.bottom + gap
  position.value = {
    left: Math.max(margin, Math.min(rect.left, window.innerWidth - width - margin)) + 'px',
    top: Math.max(margin, Math.min(top, window.innerHeight - height - margin)) + 'px',
  }
}

function outside(event: Event) {
  if (event.target instanceof Node && !panel.value?.contains(event.target) && !anchor.value?.contains(event.target)) setOpen(false, false)
}

function menuItems() {
  return Array.from(panel.value?.querySelectorAll<HTMLButtonElement>('button[role=menuitem]:not(:disabled)') ?? [])
}

function panelKeys(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    setOpen(false)
  } else if (event.key === 'Tab' && panel.value) {
    if (props.role === 'alertdialog') keepTabInside(event, panel.value)
    else {
      anchor.value?.focus()
      setOpen(false, false)
    }
  } else if (props.role === 'menu' && ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    const items = menuItems(), current = items.indexOf(document.activeElement as HTMLButtonElement)
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 :
      (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
    items[index]?.focus()
    event.preventDefault()
    event.stopPropagation()
  }
}

function removeListeners() {
  document.removeEventListener('pointerdown', outside, true)
  document.removeEventListener('focusin', outside)
  window.removeEventListener('resize', locate)
  window.removeEventListener('scroll', locate, true)
}
watch(() => props.show, async (open, previous, onCleanup) => {
  if (!open) {
    if (previous && restoreFocus && anchor.value?.isConnected) anchor.value.focus({ preventScroll: true })
    position.value = { visibility: 'hidden' }
    return
  }
  let cancelled = false
  onCleanup(() => { cancelled = true; removeListeners() })
  await nextTick()
  if (cancelled || !panel.value) return
  locate()
  await nextTick()
  if (cancelled || !panel.value) return
  const choices = props.role === 'menu' ? menuItems() : Array.from(panel.value.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'))
  const first = focusLast ? choices.at(-1) : choices[0]
  const focusTarget = first ?? panel.value
  focusTarget.focus({ preventScroll: true })
  document.addEventListener('pointerdown', outside, true)
  document.addEventListener('focusin', outside)
  window.addEventListener('resize', locate)
  window.addEventListener('scroll', locate, true)
}, { immediate: true, flush: 'post' })
onBeforeUnmount(removeListeners)
</script>

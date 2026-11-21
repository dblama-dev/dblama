<template>
  <Teleport to="body">
    <dialog v-if="show" ref="dialog" :aria-describedby="preset === 'dialog' ? description : undefined" v-bind="$attrs" class="ui-modal"
      :role="preset === 'dialog' ? 'alertdialog' : 'dialog'" aria-modal="true" :aria-labelledby="heading" tabindex="-1"
      @cancel.prevent="escape" @keydown="handleKey" @pointerdown="backdropStart" @click="backdropEnd">
      <header class="ui-modal__header">
        <h2 :id="heading">{{ title }}</h2>
        <UiButton v-if="closable" quaternary size="small" :aria-label="t('rowWorkflows.close')" @click="dismiss">
          <UiIcon name="x" :size="17" />
        </UiButton>
      </header>
      <div :id="description" class="ui-modal__body"><slot /></div>
      <footer v-if="$slots.footer || positiveText || negativeText" class="ui-modal__footer">
        <slot name="footer">
          <UiButton v-if="negativeText" data-cancel size="small" @click="answer(false)">{{ negativeText }}</UiButton>
          <UiButton v-if="positiveText" :type="type === 'warning' ? 'error' : 'primary'" size="small" @click="answer(true)">{{ positiveText }}</UiButton>
        </slot>
      </footer>
    </dialog>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import UiIcon from '../../components/UiIcon.vue'
import UiButton from '../UiButton.vue'
import { addDialog, removeDialog, keepTabInside } from './dialogFocus'

defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{
  show: boolean
  title: string
  preset?: 'card' | 'dialog'
  type?: string
  positiveText?: string
  negativeText?: string
  autoFocus?: boolean
  closable?: boolean
  closeOnEsc?: boolean
  maskClosable?: boolean
}>(), { autoFocus: true, closable: true, closeOnEsc: true, maskClosable: true })
const emit = defineEmits<{
  'update:show': [show: boolean]
  'positive-click': []
  'negative-click': []
  'after-enter': []
}>()
const { t } = useI18n()
const heading = useId()
const description = useId()
const dialog = ref<HTMLDialogElement>()
let active: HTMLDialogElement | undefined
let opener: HTMLElement | null = null
let clickedBackdrop = false

function dismiss() { emit('update:show', false) }
function answer(positive: boolean) {
  if (positive) emit('positive-click')
  else emit('negative-click')
  dismiss()
}
function escape() {
  if (props.closeOnEsc) dismiss()
}
function handleKey(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    escape()
  } else if (event.key === 'Tab' && active) {
    keepTabInside(event, active)
  }
}
function outside(event: MouseEvent) {
  if (!active || event.target !== active) return false
  const rect = active.getBoundingClientRect()
  return event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom
}
function backdropStart(event: PointerEvent) { clickedBackdrop = outside(event) }
function backdropEnd(event: MouseEvent) {
  if (clickedBackdrop && props.maskClosable && outside(event)) dismiss()
  clickedBackdrop = false
}
function release() {
  if (!active) return
  const wasTop = removeDialog(active)
  if (active.open && typeof active.close === 'function') active.close()
  active = undefined
  if (wasTop && opener?.isConnected) opener.focus({ preventScroll: true })
  opener = null
}
watch(dialog, async element => {
  release()
  if (!element) return
  active = element
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (element.isConnected) addDialog(element)
  if (element.isConnected && typeof element.showModal === 'function') element.showModal()
  else element.setAttribute('open', '')
  if (props.autoFocus) {
    const initial = element.querySelector<HTMLElement>('[autofocus], [data-cancel], .ui-modal__body :is(input:not([disabled]),textarea:not([disabled]),select:not([disabled]))')
    const focusTarget = initial ?? element
    focusTarget.focus({ preventScroll: true })
  } else element.focus({ preventScroll: true })
  await nextTick()
  if (active === element) emit('after-enter')
}, { flush: 'post' })
onBeforeUnmount(release)
</script>

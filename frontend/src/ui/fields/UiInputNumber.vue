<script setup lang="ts">
import { ref } from 'vue'
import { containerAttrs, controlAttrs, type ControlSize } from './attributes'
import UiIcon from '../../components/UiIcon.vue'
import i18n from '../../i18n'
defineOptions({ inheritAttrs: false })
const props = defineProps<{
  value?: number | null
  min?: number
  max?: number
  precision?: number
  disabled?: boolean
  readonly?: boolean
  clearable?: boolean
  size?: ControlSize
}>()
const emit = defineEmits<{ 'update:value': [value: number | null] }>()
const field = ref<HTMLInputElement>()
const { t } = i18n.global
function update(el: HTMLInputElement, finish = false) {
  if (el.value === '') {
    if (!el.validity.badInput || finish) emit('update:value', null)
    return
  }
  let value = el.valueAsNumber
  if (!Number.isFinite(value)) return
  if (finish) {
    value = Math.min(props.max ?? Infinity, Math.max(props.min ?? -Infinity, value))
    if (props.precision !== undefined) value = Number(value.toFixed(props.precision))
    el.value = String(value)
  } else if (el.validity.rangeUnderflow || el.validity.rangeOverflow || el.validity.stepMismatch) {
    return
  }
  if (value !== props.value) emit('update:value', value)
}
function clear() { emit('update:value', null); field.value?.focus() }
defineExpose({ focus: () => field.value?.focus() })
</script>

<template>
  <span class="ui-input ui-input-number" :data-size="size" :data-disabled="disabled || undefined" v-bind="containerAttrs($attrs)">
    <input ref="field" class="ui-input__control" v-bind="controlAttrs($attrs)" type="number"
      :value="value ?? ''" :min="min" :max="max" :step="precision === undefined ? 'any' : 10 ** -precision"
      :disabled="disabled" :readonly="readonly" @input="update($event.target as HTMLInputElement)"
      @blur="update($event.target as HTMLInputElement, true)">
    <button v-if="clearable && value != null && !disabled && !readonly" class="ui-input__action" type="button"
      :aria-label="t('controls.clear')" @mousedown.prevent @click="clear"><UiIcon name="x" :size="14" /></button>
    <span v-if="$slots.suffix" class="ui-input__suffix"><slot name="suffix" /></span>
  </span>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import type { InputHTMLAttributes } from 'vue'
import UiIcon from '../../components/UiIcon.vue'
import i18n from '../../i18n'
import { containerAttrs, controlAttrs, type ControlSize } from './attributes'

defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{
  value?: string | null
  type?: 'text' | 'textarea' | 'password'
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  size?: ControlSize
  clearable?: boolean
  showPasswordOn?: 'click'
  autosize?: boolean | { minRows?: number; maxRows?: number }
  inputProps?: InputHTMLAttributes
}>(), { type: 'text' })
const emit = defineEmits<{ 'update:value': [value: string] }>()
const field = ref<HTMLInputElement | HTMLTextAreaElement>()
const revealed = ref(false)
const { t } = i18n.global

function fitText() {
  const el = field.value
  if (!(el instanceof HTMLTextAreaElement) || !props.autosize || !el.offsetWidth) return
  const rule = typeof props.autosize === 'object' ? props.autosize : {}
  const style = getComputedStyle(el)
  const padding = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom)
  const line = parseFloat(style.lineHeight)
  const min = (rule.minRows ?? 1) * line + padding
  const max = (rule.maxRows ?? Infinity) * line + padding
  el.style.height = 'auto'
  el.style.height = Math.max(min, Math.min(max, el.scrollHeight)) + 'px'
}
function update(value: string) {
  emit('update:value', value)
  void nextTick(fitText)
}
function focus() { field.value?.focus() }
function clear() { update(''); focus() }
onMounted(fitText)
watch(() => [props.value, props.autosize], fitText, { flush: 'post' })
defineExpose({ focus, blur: () => field.value?.blur() })
</script>

<template>
  <span v-bind="containerAttrs($attrs)" class="ui-input" :data-size="size" :data-disabled="disabled || undefined">
    <span v-if="$slots.prefix" class="ui-input__prefix"><slot name="prefix" /></span>
    <textarea v-if="type === 'textarea'" ref="field" v-bind="{ ...controlAttrs($attrs), ...inputProps }"
      class="ui-input__control" :value="value ?? ''" :disabled="disabled" :readonly="readonly" :placeholder="placeholder"
      :rows="typeof autosize === 'object' ? autosize.minRows : undefined"
      @input="update(($event.target as HTMLTextAreaElement).value)" />
    <input v-else ref="field" v-bind="{ ...controlAttrs($attrs), ...inputProps }" class="ui-input__control"
      :type="type === 'password' && !revealed ? 'password' : 'text'" :value="value ?? ''"
      :disabled="disabled" :readonly="readonly" :placeholder="placeholder" @input="update(($event.target as HTMLInputElement).value)">
    <button v-if="clearable && value && !disabled && !readonly" class="ui-input__action" type="button"
      :aria-label="t('controls.clear')" @mousedown.prevent @click="clear"><UiIcon name="x" :size="14" /></button>
    <button v-if="type === 'password' && showPasswordOn" class="ui-input__action" type="button"
      :disabled="disabled" :aria-label="t(revealed ? 'controls.hidePassword' : 'controls.showPassword')"
      :aria-pressed="revealed" @mousedown.prevent @click="revealed = !revealed"><UiIcon name="eye" :size="16" /></button>
    <span v-if="$slots.suffix" class="ui-input__suffix"><slot name="suffix" /></span>
  </span>
</template>

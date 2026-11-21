<script setup lang="ts">
import { containerAttrs, controlAttrs, type ControlSize } from './attributes'
defineOptions({ inheritAttrs: false })
defineProps<{ checked?: boolean; disabled?: boolean; label?: string; indeterminate?: boolean; size?: ControlSize }>()
const emit = defineEmits<{ 'update:checked': [checked: boolean] }>()
</script>

<template>
  <label v-bind="containerAttrs($attrs)" class="ui-checkbox" :data-disabled="disabled || undefined">
    <input v-bind="controlAttrs($attrs)" type="checkbox" :checked="checked" :disabled="disabled"
      :indeterminate.prop="indeterminate ?? false" @change="emit('update:checked', ($event.target as HTMLInputElement).checked)">
    <span v-if="label || $slots.default" class="ui-checkbox__label"><slot>{{ label }}</slot></span>
  </label>
</template>

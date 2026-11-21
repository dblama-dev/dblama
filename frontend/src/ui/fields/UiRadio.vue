<script setup lang="ts">
import { inject } from 'vue'
import { radioGroupKey } from './radio'
import { containerAttrs, controlAttrs, type OptionValue } from './attributes'
defineOptions({ inheritAttrs: false })
defineProps<{ value: OptionValue; disabled?: boolean }>()
const group = inject(radioGroupKey)
</script>
<template>
  <label class="ui-radio" v-bind="containerAttrs($attrs)" :data-disabled="disabled || group?.disabled || undefined">
    <input v-bind="controlAttrs($attrs)" type="radio" :name="group?.name" :value="value" :checked="value === group?.value"
      :disabled="disabled || group?.disabled" @change="group?.choose(value)">
    <span><slot /></span>
  </label>
</template>

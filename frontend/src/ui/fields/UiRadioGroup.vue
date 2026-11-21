<script setup lang="ts" generic="T extends string | number">
import { provide, useId } from 'vue'
import { radioGroupKey } from './radio'
const props = defineProps<{ value?: T; disabled?: boolean; name?: string }>()
const emit = defineEmits<{ 'update:value': [value: T] }>()
const id = useId()
provide(radioGroupKey, {
  get name() { return props.name ?? id },
  get value() { return props.value },
  get disabled() { return Boolean(props.disabled) },
  choose(value) { emit('update:value', value as T) },
})
</script>
<template><span class="ui-radio-group" role="radiogroup"><slot /></span></template>

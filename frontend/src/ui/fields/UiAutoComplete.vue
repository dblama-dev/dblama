<script setup lang="ts">
import { useId } from 'vue'
import { containerAttrs, controlAttrs } from './attributes'
defineOptions({ inheritAttrs: false })
defineProps<{ options?: string[]; value?: string | null; disabled?: boolean }>()
const emit = defineEmits<{ 'update:value': [value: string] }>()
const listId = useId()
</script>
<template>
  <span v-bind="containerAttrs($attrs)" class="ui-input ui-autocomplete" :data-disabled="disabled || undefined">
    <input v-bind="controlAttrs($attrs)" class="ui-input__control" :list="listId" :value="value ?? ''" :disabled="disabled"
      @input="emit('update:value', ($event.target as HTMLInputElement).value)">
    <datalist :id="listId"><option v-for="item in options" :key="item" :value="item" /></datalist>
  </span>
</template>

<template>
  <UiFloatingPanel v-model:show="open" role="menu" class="ui-dropdown" :trigger-slot="$slots.default">
    <button v-for="option in options" :key="option.key" type="button" role="menuitem" tabindex="-1"
      class="ui-dropdown-option" :disabled="option.disabled" @click="select(option.key)">
      {{ option.label }}
    </button>
  </UiFloatingPanel>
</template>

<script setup lang="ts" generic="Key extends string">
import { ref } from 'vue'
import UiFloatingPanel from './UiFloatingPanel.vue'

defineProps<{ options: readonly { key: Key; label: string; disabled?: boolean }[] }>()
const emit = defineEmits<{ select: [key: Key] }>()
const open = ref(false)
function select(key: Key) {
  open.value = false
  emit('select', key)
}
</script>

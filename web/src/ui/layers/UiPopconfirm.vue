<template>
  <UiFloatingPanel :show="visible" role="alertdialog" :aria-label="t('objects.confirm')" :aria-describedby="messageId"
    class="ui-popconfirm" :trigger-slot="$slots.trigger" @update:show="setOpen">
    <p :id="messageId"><slot /></p>
    <div class="ui-popconfirm__actions">
      <UiButton size="small" @click="answer(false)">{{ t('row.cancel') }}</UiButton>
      <UiButton size="small" type="primary" @click="answer(true)">{{ t('objects.confirm') }}</UiButton>
    </div>
  </UiFloatingPanel>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import UiFloatingPanel from './UiFloatingPanel.vue'
import UiButton from '../UiButton.vue'

const props = withDefaults(defineProps<{ show?: boolean }>(), { show: undefined })
const emit = defineEmits<{ 'update:show': [value: boolean]; 'positive-click': []; 'negative-click': [] }>()
const { t } = useI18n()
const local = ref(false)
const visible = computed(() => props.show ?? local.value)
const messageId = useId()

function setOpen(value: boolean) {
  local.value = value
  emit('update:show', value)
}
function answer(positive: boolean) {
  if (positive) emit('positive-click')
  else emit('negative-click')
  setOpen(false)
}
</script>

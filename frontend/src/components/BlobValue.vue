<template>
  <span>
    <code v-if="mode === 'hex'" class="datagrid__hex" :title="hex">{{ hex }}</code>
    <span v-else-if="mode === 'text'" class="binary-text">{{ preview.text }}</span>
    <img v-else-if="preview.image" :src="preview.image" alt="BLOB" class="binary-image" />
    <button v-if="preview.text !== null || preview.image" type="button" class="binary-toggle" @click="toggle">{{ mode === 'hex' ? (preview.image ? t('grid.preview') : t('grid.text')) : 'HEX' }}</button>
  </span>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { binaryPreview } from '../lib/binaryPreview'
const props = defineProps<{hex:string}>()
const { t } = useI18n()
const preview = computed(() => binaryPreview(props.hex))
const mode = ref<'hex'|'text'|'image'>('hex')
function toggle(): void { mode.value = mode.value === 'hex' ? (preview.value.image ? 'image' : 'text') : 'hex' }
</script>
<style scoped>
.datagrid__hex { display: inline-block; max-width: 240px; overflow: hidden; text-overflow: ellipsis; vertical-align: middle; }
.binary-toggle { margin-left: 6px; border: 0; cursor: pointer; color: inherit; background: transparent; text-decoration: underline; }
.binary-text { white-space: pre-wrap; overflow-wrap: anywhere; }
.binary-image { max-width: 240px; max-height: 160px; object-fit: contain; }
</style>

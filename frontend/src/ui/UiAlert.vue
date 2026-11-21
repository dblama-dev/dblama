<script setup lang="ts">
withDefaults(defineProps<{ type?: string; title?: string }>(), { type: 'info' })
</script>
<template>
  <div class="ui-alert" :class="'ui-alert--' + type" :role="type === 'error' || type === 'warning' ? 'alert' : 'status'">
    <strong v-if="title">{{ title }}</strong>
    <div><slot /></div>
  </div>
</template>

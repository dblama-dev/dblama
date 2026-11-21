<template>
  <header class="db-page-head tool-heading">
    <div class="db-heading">
      <span class="db-heading-icon"><UiIcon :name="icon" :size="23" /></span>
      <h1 class="db-title">{{ title }}</h1>
    </div>
    <div v-if="$slots.default" class="tool-actions"><slot /></div>
  </header>
</template>

<script setup lang="ts">
import UiIcon from './UiIcon.vue'
defineProps<{ title: string; icon: string }>()
</script>

<style src="./toolWorkspaces.css"></style>

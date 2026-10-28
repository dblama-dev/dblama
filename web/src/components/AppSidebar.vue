<template>
  <aside class="app-sidebar">
    <ObjectTree />
    <AdvancedNavigation />
  </aside>
</template>

<script setup lang="ts">
import AdvancedNavigation from './AdvancedNavigation.vue'
import ObjectTree from './ObjectTree.vue'
</script>

<style scoped>
.app-sidebar {
  height: 100%;
  overflow: auto;
}
</style>

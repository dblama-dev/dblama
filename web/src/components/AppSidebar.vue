<template>
  <aside class="app-sidebar">
    <ObjectTree>
      <template #footer="{ db }"><AdvancedNavigation :db="db" /></template>
    </ObjectTree>
  </aside>
</template>
<script setup lang="ts">
import AdvancedNavigation from './AdvancedNavigation.vue'
import ObjectTree from './ObjectTree.vue'
</script>
<style scoped>
.app-sidebar { flex: 1; min-height: 0; }
</style>

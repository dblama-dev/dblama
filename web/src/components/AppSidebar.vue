<template>
  <aside class="app-sidebar"><AdvancedNavigation /><ObjectTree /></aside>
</template>
<script setup lang="ts">
import AdvancedNavigation from './AdvancedNavigation.vue'
import ObjectTree from './ObjectTree.vue'
</script>
<style scoped>
.app-sidebar{height:calc(100% - 44px);overflow:auto;}
</style>

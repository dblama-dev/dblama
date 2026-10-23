<template>
  <n-layout class="basic-layout" has-sider>
    <n-layout-sider bordered :width="220">
      <AppSidebar />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered>
        <AppTopbar />
      </n-layout-header>
      <n-layout-content class="basic-layout__content">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { NLayout, NLayoutContent, NLayoutHeader, NLayoutSider } from 'naive-ui'

import AppSidebar from '../components/AppSidebar.vue'
import AppTopbar from '../components/AppTopbar.vue'
</script>

<style scoped>
.basic-layout {
  height: 100vh;
}

.basic-layout__content {
  padding: 16px;
}
</style>

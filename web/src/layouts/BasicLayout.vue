<template>
  <n-layout class="basic-layout" has-sider>
    <n-layout-sider bordered :width="preferences.width" :collapsed="preferences.collapsed" :collapsed-width="44" collapse-mode="width">
      <div class="basic-layout__sidebar-controls">
        <n-button size="small" :aria-label="t(preferences.collapsed?'navigationWorkflows.expand':'navigationWorkflows.collapse')" @click="preferences.collapsed = !preferences.collapsed">{{ preferences.collapsed ? '☰' : '‹' }}</n-button>
        <input v-if="!preferences.collapsed" v-model.number="preferences.width" type="range" min="180" max="420" step="10" :aria-label="t('navigationWorkflows.sidebarWidth')" />
      </div>
      <AppSidebar v-if="!preferences.collapsed" />
    </n-layout-sider>
    <n-layout class="basic-layout__main">
      <n-layout-header bordered><AppTopbar /></n-layout-header>
      <n-layout-content class="basic-layout__content"><router-view /></n-layout-content>
    </n-layout>
  </n-layout>
</template>
<script setup lang="ts">
import {ref,watch} from 'vue'
import {useI18n} from 'vue-i18n'
import { NButton,NLayout, NLayoutContent, NLayoutHeader, NLayoutSider } from 'naive-ui'
import AppSidebar from '../components/AppSidebar.vue'
import AppTopbar from '../components/AppTopbar.vue'
import {readNavigationPreferences,writeNavigationPreferences} from '../lib/navigationPreferences'
const {t}=useI18n()
const preferences=ref(readNavigationPreferences())
if(window.innerWidth<720) preferences.value.collapsed=true
watch(()=>[preferences.value.width,preferences.value.collapsed],()=>writeNavigationPreferences(undefined,{...readNavigationPreferences(),width:preferences.value.width,collapsed:preferences.value.collapsed}))
</script>
<style scoped>
.basic-layout {height:100vh;}.basic-layout__main {min-width:0;}.basic-layout__content{padding:16px;max-width:100%;box-sizing:border-box;}.basic-layout__sidebar-controls{display:flex;align-items:center;gap:6px;padding:6px;}.basic-layout__sidebar-controls input{width:140px;min-width:0;}
</style>

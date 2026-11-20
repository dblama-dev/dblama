<template>
  <n-layout class="basic-layout" has-sider @keydown.esc="closeMobilePanel">
    <n-layout-sider bordered class="basic-layout__sider" :width="siderWidth" :collapsed="preferences.collapsed" :collapsed-width="44"
      collapse-mode="width" :class="{ 'basic-layout__overlay': mobilePanelOpen }">
      <div class="basic-layout__sidebar">
        <div class="basic-layout__sidebar-controls">
          <router-link v-if="!preferences.collapsed" to="/server">{{ t('server.title') }}</router-link>
          <n-button size="small" quaternary :aria-expanded="!preferences.collapsed"
            :aria-label="toggleLabel" :title="toggleLabel" @click="preferences.collapsed = !preferences.collapsed">
            {{ preferences.collapsed ? '☰' : '‹' }}
          </n-button>
        </div>
        <AppSidebar v-if="!preferences.collapsed" :id="sidebarID" />
        <div v-if="!preferences.collapsed && !narrow" class="basic-layout__resize"
          role="separator" tabindex="0" aria-orientation="vertical" :aria-controls="sidebarID" :aria-label="t('navigationWorkflows.sidebarWidth')"
          :aria-valuemin="minWidth" :aria-valuemax="maxWidth" :aria-valuenow="preferences.width"
          @keydown="resizeWithKeys" @pointerdown="startResize" @pointermove="resize"
          @lostpointercapture="drag = null" />
      </div>
    </n-layout-sider>
    <div v-if="mobilePanelOpen" class="basic-layout__backdrop" aria-hidden="true" @click="closeMobilePanel" />
    <n-layout class="basic-layout__main" :inert="mobilePanelOpen">
      <n-layout-header bordered><AppTopbar /></n-layout-header>
      <n-layout-content class="basic-layout__content"><router-view /></n-layout-content>
    </n-layout>
  </n-layout>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { NButton, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider } from 'naive-ui'
import AppSidebar from '../components/AppSidebar.vue'
import AppTopbar from '../components/AppTopbar.vue'
import { readNavigationPreferences, writeNavigationPreferences } from '../lib/navigationPreferences'
const { t } = useI18n()
const route = useRoute()
const minWidth = 180
const maxWidth = 420
const preferences = ref(readNavigationPreferences())
const sidebarID = useId()
const viewportWidth = ref(window.innerWidth)
const narrow = computed(() => viewportWidth.value < 720)
const siderWidth = computed(() => narrow.value ? Math.min(preferences.value.width, viewportWidth.value - 36) : preferences.value.width)
if (narrow.value) preferences.value.collapsed = true
const mobilePanelOpen = computed(() => narrow.value && !preferences.value.collapsed)
const toggleLabel = computed(() => t(preferences.value.collapsed ? 'navigationWorkflows.expand' : 'navigationWorkflows.collapse'))

function closeMobilePanel(): void {
  if (mobilePanelOpen.value) preferences.value.collapsed = true
}
watch(() => route.fullPath, closeMobilePanel)
function updateViewport(): void {
  const previouslyNarrow = narrow.value
  viewportWidth.value = window.innerWidth
  if (narrow.value !== previouslyNarrow) preferences.value.collapsed = narrow.value || readNavigationPreferences().collapsed
}
onMounted(() => window.addEventListener('resize', updateViewport))
onBeforeUnmount(() => window.removeEventListener('resize', updateViewport))
watch(() => [preferences.value.width, preferences.value.collapsed], () => {
  if (!narrow.value) writeNavigationPreferences(undefined, { ...readNavigationPreferences(), width: preferences.value.width, collapsed: preferences.value.collapsed })
})

const drag = ref<{ start: number; width: number } | null>(null)
function startResize(event: PointerEvent): void {
  if (event.button !== 0) return
  drag.value = { start: event.clientX, width: preferences.value.width }
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
}
function setWidth(width: number): void {
  preferences.value.width = Math.min(maxWidth, Math.max(minWidth, width))
}
function resize(event: PointerEvent): void {
  if (drag.value) setWidth(drag.value.width + event.clientX - drag.value.start)
}
function resizeWithKeys(event: KeyboardEvent): void {
  const width = preferences.value.width
  const widths: Record<string, number> = { Home: minWidth, End: maxWidth, ArrowLeft: width - 10, ArrowRight: width + 10 }
  if (!Object.hasOwn(widths, event.key)) return
  event.preventDefault()
  setWidth(widths[event.key]!)
}
</script>
<style scoped>
.basic-layout { height: 100vh; height: 100dvh; }
.basic-layout__sider { transition-property: background-color, color; }
.basic-layout__main { min-width: 0; }
.basic-layout__content { padding: 16px; max-width: 100%; box-sizing: border-box; }
.basic-layout__sidebar { position: relative; display: flex; flex-direction: column; height: 100%; }
.basic-layout__sidebar-controls { display: flex; align-items: center; justify-content: space-between; gap: 4px; padding: 6px 8px; border-bottom: 1px solid #8884; }
.basic-layout__sidebar-controls a { color: inherit; font-weight: 600; text-decoration: none; }
.basic-layout__sidebar-controls a:hover { text-decoration: underline; }
.basic-layout__resize { position: absolute; inset: 0 0 0 auto; width: 5px; cursor: col-resize; touch-action: none; z-index: 2; }
.basic-layout__resize:hover, .basic-layout__resize:focus-visible { background: #8886; }
.basic-layout__resize:focus-visible { outline: 1px solid currentColor; outline-offset: -1px; }
.basic-layout__overlay { position: absolute; z-index: 5; inset: 0 auto 0 0; max-width: calc(100vw - 36px); }
.basic-layout__backdrop { position: absolute; z-index: 4; inset: 0; background: rgb(0 0 0 / 35%); }
@media (max-width: 719px) { .basic-layout__content { padding: 10px; } }
</style>

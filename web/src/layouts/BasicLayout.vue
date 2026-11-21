<template>
  <div class="basic-layout" @keydown.esc="closeMobilePanel">
    <aside v-if="!preferences.collapsed" class="basic-layout__sider" :style="{ width: siderWidth + 'px' }"
      :class="{ 'basic-layout__overlay': mobilePanelOpen }">
      <div class="basic-layout__sidebar" :id="sidebarID">
        <header class="basic-layout__brandbar">
          <router-link class="brand" to="/server" :title="t('server.title')">
            <span class="brand__mark"><UiIcon name="stack" :size="21" /></span><span class="brand__name">DB Manager</span>
          </router-link>
          <button ref="closeButton" class="panel-toggle" type="button" :aria-label="toggleLabel" :title="toggleLabel"
            :aria-controls="sidebarID" aria-expanded="true" @click="togglePanel"><UiIcon :name="narrow ? 'x' : 'panel'" :size="16" /></button>
        </header>
        <AppSidebar />
        <div v-if="!narrow" class="basic-layout__resize"
          role="separator" tabindex="0" aria-orientation="vertical" :aria-controls="sidebarID" :aria-label="t('navigationWorkflows.sidebarWidth')"
          :aria-valuemin="minWidth" :aria-valuemax="maxWidth" :aria-valuenow="preferences.width"
          @keydown="resizeWithKeys" @pointerdown="startResize" @pointermove="resize"
          @lostpointercapture="drag = null" />
      </div>
    </aside>
    <div v-if="mobilePanelOpen" class="basic-layout__backdrop" aria-hidden="true" @click="closeMobilePanel" />
    <div class="basic-layout__main" :inert="mobilePanelOpen">
      <header class="basic-layout__top">
        <button v-if="preferences.collapsed" ref="openButton" class="panel-toggle" type="button" :aria-label="toggleLabel" :title="toggleLabel"
          aria-expanded="false" @click="togglePanel"><UiIcon name="panel" :size="18" /></button>
        <AppTopbar />
      </header>
      <div class="basic-layout__content"><main class="basic-layout__workspace"><router-view /></main></div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppSidebar from '../components/AppSidebar.vue'
import AppTopbar from '../components/AppTopbar.vue'
import UiIcon from '../components/UiIcon.vue'
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

const openButton = ref<HTMLButtonElement>()
const closeButton = ref<HTMLButtonElement>()
async function togglePanel(): Promise<void> {
  preferences.value.collapsed = !preferences.value.collapsed
  await nextTick()
  const target = preferences.value.collapsed ? openButton : closeButton
  target.value?.focus({ preventScroll: true })
}
function closeMobilePanel(): void {
  if (mobilePanelOpen.value) void togglePanel()
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
.basic-layout { display:flex; overflow:hidden; height: 100vh; height: 100dvh; color: var(--db-ink); background: var(--db-bg); }
.basic-layout__sider { flex:none; height:100%; transition-property: background-color, color; background: var(--db-sidebar); }
.basic-layout__sidebar { box-sizing: border-box; position: relative; display: flex; flex-direction: column; height: 100%; border-right: 1px solid var(--db-line); }
.basic-layout__brandbar, .basic-layout__top { display: flex; align-items: center; flex-shrink: 0; }
.basic-layout__brandbar { height: 60px; box-sizing: border-box; gap: 5px; padding: 0 10px 0 15px; background: var(--db-sidebar); }
.brand { display: flex; align-items: center; gap: 9px; min-width: 0; flex: 1; text-decoration: none; color: var(--db-ink); }
.brand__name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; letter-spacing: -.85px; font-size: 19px; font-weight: 700; }
.brand__mark { display: grid; place-items: center; flex-shrink: 0; width: 31px; height: 31px; border-radius: 9px; color: white; background: linear-gradient(145deg,#637af3,#4055d7); box-shadow: inset 0 1px 0 #ffffff36; }
.panel-toggle { display: inline-grid; place-items: center; flex: 0 0 28px; height: 28px; padding: 0; border-radius: 6px; border: 0; cursor: pointer; color: var(--db-muted); background: transparent; }
.panel-toggle:hover, .panel-toggle:focus-visible { color: var(--db-accent); background: var(--db-tint); }
.basic-layout__main { display:flex; flex-direction:column; flex:1; min-width: 0; background: var(--db-bg); }
.basic-layout__top { box-sizing: border-box; gap: 10px; padding: 0 clamp(18px, 2.6vw, 42px); background: var(--db-panel); border-bottom: 1px solid var(--db-line); }
.basic-layout__content { overflow:auto; box-sizing: border-box; flex: 1; min-height: 0; max-width: 100%; background: var(--db-bg); }
.basic-layout__workspace { box-sizing: border-box; width: 100%; max-width: 1550px; margin: 0 auto; padding: 25px clamp(18px, 2.6vw, 42px) 36px; }
.basic-layout__resize { position: absolute; inset: 0 -1px 0 auto; width: 5px; cursor: col-resize; touch-action: none; z-index: 2; }
.basic-layout__resize:hover, .basic-layout__resize:focus-visible { background: var(--db-accent); opacity: .45; }
.basic-layout__overlay { position: absolute; z-index: 5; inset: 0 auto 0 0; max-width: calc(100vw - 36px); box-shadow: 4px 0 30px #17213b28; }
.basic-layout__backdrop { position: absolute; z-index: 4; inset: 0; background: rgb(22 30 50 / 30%); }
@media (max-width: 719px) {
  .basic-layout__workspace { padding: 16px 12px 22px; }
  .basic-layout__top { padding: 0 7px; gap: 4px; }
  .basic-layout__brandbar { height: 49px; }
}
</style>

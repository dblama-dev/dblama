<template>
  <section class="catalog-header db-panel">
    <div class="catalog-header__main">
      <div class="catalog-header__copy">
        <span class="catalog-header__scope"><UiIcon :name="icon" :size="15" />{{ scope }}</span>
        <h1>{{ title }}</h1>
        <div v-if="engine || detail" class="catalog-header__meta">
          <span v-if="engine" class="catalog-header__engine">{{ engine }}</span>
          <span v-if="detail" class="catalog-header__detail">{{ detail }}</span>
        </div>
      </div>
    </div>
    <dl v-if="stats?.length" class="catalog-header__stats">
      <div v-for="stat in stats" :key="stat.label" :class="{ 'catalog-header__identity': stat.icon === 'user' }">
        <span class="catalog-header__glyph"><UiIcon :name="stat.icon" :size="17" /></span>
        <div><dt>{{ stat.label }}</dt><dd>{{ stat.value }}</dd></div>
      </div>
    </dl>
  </section>
</template>
<script setup lang="ts">
import UiIcon from './UiIcon.vue'
defineProps<{
  title: string
  scope: string
  icon: string
  engine?: string
  detail?: string
  stats?: { label: string; icon: string; value: string }[]
}>()
</script>
<style scoped>
.catalog-header { overflow: hidden; }
.catalog-header__main {
  position: relative; isolation: isolate; display: flex; align-items: center; min-height: 145px;
  background: linear-gradient(105deg, color-mix(in srgb, var(--db-tint) 44%, var(--db-panel)), var(--db-tint));
}
.catalog-header__main::after {
  content: ''; opacity: var(--db-art-opacity, 1); position: absolute; inset: 0 0 0 auto; width: min(43%,430px); z-index: -1;
  background: url('../assets/storage.svg') right center / auto 103% no-repeat; pointer-events: none;
}
.catalog-header__copy { padding: 21px 28px; max-width: 67%; }
.catalog-header__scope, .catalog-header__meta { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--db-muted); }
.catalog-header h1 { margin: 3px 0 8px; font-size: clamp(28px,3.2vw,43px); line-height: 1.15; font-weight: 620; letter-spacing: -.05em; overflow-wrap: anywhere; }
.catalog-header__meta { flex-wrap: wrap; }
.catalog-header__engine { border: 1px solid var(--db-line); background: var(--db-panel); border-radius: 5px; padding: 1px 7px; font-weight: 550; }
.catalog-header__detail { font-family: var(--db-mono); overflow-wrap: anywhere; }
.catalog-header__stats { display: flex; margin: 0; border-top: 1px solid var(--db-line); padding: 13px 0; }
.catalog-header__stats > div { flex: 1; min-width: 0; display: flex; align-items: center; gap: 11px; padding: 0 21px; }
.catalog-header__stats > div + div { border-left: 1px solid var(--db-line); }
.catalog-header__stats dt { color: var(--db-muted); font-size: 12px; }
.catalog-header__stats dd { margin: 1px 0 0; line-height: 1.25; font-size: 18px; font-weight: 600; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.catalog-header__glyph { display: grid; place-items: center; width: 32px; height: 32px; color: var(--db-muted); background: var(--db-sidebar); border: 1px solid var(--db-line); border-radius: 7px; }
@media(max-width:719px) {
  .catalog-header__main { min-height: 115px; }
  .catalog-header__main::after { width: 34%; background-size: auto 88%; background-position: 60% center; opacity: min(.67, var(--db-art-opacity, 1)); }
  .catalog-header__copy { padding: 16px; max-width: 74%; }
  .catalog-header__glyph { display: none; }
  .catalog-header__stats > div { padding: 0 10px; }
  .catalog-header__stats > .catalog-header__identity { flex: 1.6; }
  .catalog-header__stats dt { font-size: 11px; }
  .catalog-header__stats dd { font-size: 15px; }
}
</style>

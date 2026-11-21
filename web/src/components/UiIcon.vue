<template>
  <svg class="ui-icon" :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor"
    stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <g v-if="name === 'db'"><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0"/></g>
    <g v-else-if="name === 'stack'"><path d="m3 7 9-4 9 4-9 4-9-4ZM3 12l9 4 9-4M3 17l9 4 9-4"/></g>
    <g v-else-if="name === 'schema'"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/><path d="M6.5 10v7.5H14"/></g>
    <g v-else-if="name === 'overview'"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></g>
    <g v-else-if="name === 'table'"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M9 10v10"/></g>
    <g v-else-if="name === 'columns'"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16m6-16v16"/></g>
    <g v-else-if="name === 'rows'"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9.5h18M3 15h18"/></g>
    <g v-else-if="name === 'disk'"><path d="m5.5 4-3 11v3.5A1.5 1.5 0 0 0 4 20h16a1.5 1.5 0 0 0 1.5-1.5V15l-3-11zM3 14h18m-5 3h.01m2.5 0h.01"/></g>
    <g v-else-if="name === 'panel'"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></g>
    <g v-else-if="name === 'search'"><circle cx="10.5" cy="10.5" r="6.8"/><path d="m16 16 4.5 4.5"/></g>
    <g v-else-if="name === 'code'"><path d="m8 7-5 5 5 5m8-10 5 5-5 5m-3-13-2 16"/></g>
    <g v-else-if="name === 'terminal'"><rect x="2" y="4" width="20" height="16" rx="2.5"/><path d="m6.5 9 3 3-3 3m6 0h4"/></g>
    <g v-else-if="name === 'clock'"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/></g>
    <g v-else-if="name === 'history'"><path d="M3 10a9 9 0 1 1 1 6M3 4v6h6m3-3v5l3 2"/></g>
    <g v-else-if="name === 'link'"><path d="m10 13 4-4m-6 6-1 1a3.5 3.5 0 0 1-5-5l4-4a3.5 3.5 0 0 1 5 0m2 10a3.5 3.5 0 0 0 5 0l4-4a3.5 3.5 0 0 0-5-5l-1 1"/></g>
    <g v-else-if="name === 'network'"><rect x="9" y="3" width="6" height="5" rx="1"/><rect x="2" y="16" width="6" height="5" rx="1"/><rect x="16" y="16" width="6" height="5" rx="1"/><path d="M12 8v4m-7 4v-4h14v4"/></g>
    <g v-else-if="name === 'star'"><path d="m12 3 2.8 5.8 6.4.9-4.6 4.5 1.1 6.4-5.7-3-5.7 3 1.1-6.4L2.8 9.7l6.4-.9Z"/></g>
    <g v-else-if="name === 'plus'"><path d="M12 5v14M5 12h14"/></g>
    <g v-else-if="name === 'x'"><path d="m6 6 12 12M18 6 6 18"/></g>
    <g v-else-if="name === 'chevron-left'"><path d="m15 6-6 6 6 6"/></g>
    <g v-else-if="name === 'chevron-right'"><path d="m9 6 6 6-6 6"/></g>
    <g v-else-if="name === 'chevron-down'"><path d="m6 9 6 6 6-6"/></g>
    <g v-else-if="name === 'chevrons-up-down'"><path d="m8 9 4-4 4 4m-8 6 4 4 4-4"/></g>
    <g v-else-if="name === 'arrow-up'"><path d="M12 20V4m-6 6 6-6 6 6"/></g>
    <g v-else-if="name === 'arrow-down'"><path d="M12 4v16m-6-6 6 6 6-6"/></g>
    <g v-else-if="name === 'arrow-right'"><path d="M4 12h16m-6-6 6 6-6 6"/></g>
    <g v-else-if="name === 'arrow-up-right'"><path d="M6 18 18 6M6 6h12v12"/></g>
    <g v-else-if="name === 'sort'"><path d="M4 6h16M4 12h11M4 18h6"/></g>
    <g v-else-if="name === 'filter'"><path d="M3 6h18M6 12h12m-8 6h4"/></g>
    <g v-else-if="name === 'key'"><circle cx="8" cy="15" r="4.5"/><path d="m11.5 11.5 8-8m-4 4 2.5 2.5m-.5-5L20 7"/></g>
    <g v-else-if="name === 'download'"><path d="M12 3v12m-5-5 5 5 5-5M4 15v5h16v-5"/></g>
    <g v-else-if="name === 'edit'"><path d="m15 5 4 4M4 16l12-12a2 2 0 0 1 4 4L8 20l-5 1Z"/></g>
    <g v-else-if="name === 'check'"><path d="m4 12 5 5L20 6"/></g>
    <g v-else-if="name === 'play'"><path d="m7 4 13 8-13 8Z"/></g>
    <g v-else-if="name === 'copy'"><rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V4a1 1 0 0 0-1-1H4a1 1 0 0 0-1 1v11a1 1 0 0 0 1 1h4"/></g>
    <g v-else-if="name === 'keyboard'"><rect x="2" y="5" width="20" height="14" rx="2"/><path d="M6 9h.1m4 0h.1m4 0h.1m4 0h.1M6 12h.1m4 0h.1m4 0h.1m4 0h.1M8 15h8"/></g>
    <g v-else-if="name === 'server'"><rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01"/></g>
    <g v-else-if="name === 'folder'"><path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z"/></g>
    <g v-else-if="name === 'info'"><circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/></g>
    <g v-else-if="name === 'log-in'"><path d="M14 4h6v16h-6M3 12h12m-5-5 5 5-5 5"/></g>
    <g v-else-if="name === 'user'"><circle cx="12" cy="8" r="4"/><path d="M4 21v-2a7 7 0 0 1 7-6h2a7 7 0 0 1 7 6v2"/></g>
    <g v-else-if="name === 'moon'"><path d="M20.8 13a8.5 8.5 0 0 1-10-10 9 9 0 1 0 10 10Z"/></g>
    <g v-else-if="name === 'sun'"><path d="M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4m0-12.8L17 7M7 17l-1.4 1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z"/></g>
    <g v-else-if="name === 'logout'"><path d="M10 4H4v16h6m4-12 4 4-4 4m-5-4h10"/></g>
    <g v-else-if="name === 'eye'"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"/></g>
    <g v-else-if="name === 'settings'"><path d="M4 7h8m4 0h4M4 17h3m4 0h9M16 7a2 2 0 1 1-4 0 2 2 0 0 1 4 0Zm-5 10a2 2 0 1 1-4 0 2 2 0 0 1 4 0Z"/></g>
    <g v-else-if="name === 'activity'"><path d="M2 12h5l3-8 4 16 3-8h5"/></g>
    <g v-else-if="name === 'upload'"><path d="M12 16V3m-5 5 5-5 5 5M4 16v4h16v-4"/></g>
    <g v-else-if="name === 'bell'"><path d="M4.5 17 6 14v-4a6 6 0 1 1 12 0v4l1.5 3h-15ZM10 20a2 2 0 0 0 4 0"/></g>
    <g v-else-if="name === 'globe'"><path d="M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0ZM3 12h18m-9-9c-4 4-4 14 0 18 4-4 4-14 0-18Z"/></g>
    <g v-else-if="name === 'refresh'"><path d="M20 10a8 8 0 0 0-14-4L3 9m0-5v5h5m-4 5a8 8 0 0 0 14 4l3-3m0 5v-5h-5"/></g>
    <g v-else-if="name === 'box'"><path d="m12 3 9 5-9 5-9-5 9-5Zm9 5v9l-9 5-9-5V8m9 5v9"/></g>
    <g v-else-if="name === 'more'"><circle cx="5" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="19" cy="12" r="1" fill="currentColor"/></g>
  </svg>
</template>
<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number | string }>(), { size: 16 })
</script>
<style scoped>
.ui-icon { flex-shrink: 0; vertical-align: middle; }
</style>

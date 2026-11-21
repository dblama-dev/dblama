<script setup lang="ts">
withDefaults(defineProps<{
  type?: string
  attrType?: 'button' | 'submit' | 'reset'
  size?: string
  tag?: 'button' | 'a'
  href?: string
  loading?: boolean
  disabled?: boolean
  secondary?: boolean
  quaternary?: boolean
  block?: boolean
}>(), { type: 'default', size: 'medium', tag: 'button', attrType: 'button' })
</script>
<template>
  <component :is="tag" class="ui-button"
    :class="['ui-button--' + size, 'ui-button--' + type, { 'ui-button--quiet': quaternary, 'ui-button--secondary': secondary, 'ui-button--block': block }]"
    :type="tag === 'button' ? attrType : undefined" :disabled="tag === 'button' && (disabled || loading)"
    :href="tag === 'a' && !(disabled || loading) ? href : undefined"
    :aria-busy="loading || undefined" :aria-disabled="disabled || loading || undefined">
    <span v-if="loading" class="ui-spinner" aria-hidden="true"></span>
    <slot v-else name="icon" /><slot />
  </component>
</template>

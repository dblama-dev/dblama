<template>
  <slot />
  <Teleport v-if="notices.length" :to="topDialog ?? 'body'">
    <div class="ui-notices">
      <section v-for="notice in notices" :key="notice.id" class="ui-notification" :class="'ui-notification--' + notice.kind"
        :role="notice.kind === 'error' ? 'alert' : 'status'">
        <UiIcon :name="notice.kind === 'error' ? 'info' : 'check'" :size="17" />
        <div><strong v-if="notice.title">{{ notice.title }}</strong><p>{{ notice.content }}</p></div>
        <UiButton quaternary size="small" :aria-label="t('rowWorkflows.close')" @click="remove(notice.id)"><UiIcon name="x" :size="15" /></UiButton>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { onBeforeUnmount, provide, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import UiIcon from '../../components/UiIcon.vue'
import UiButton from '../UiButton.vue'
import { topDialog } from './dialogFocus'
import { notificationKey, type Notice } from './notifications'

const { t } = useI18n()
type Kind = 'success' | 'error'
const notices = ref<(Notice & { kind: Kind; id: number })[]>([])
const timers = new Map<number, ReturnType<typeof setTimeout>>()
let sequence = 0

function remove(id: number) {
  clearTimeout(timers.get(id))
  timers.delete(id)
  notices.value = notices.value.filter(item => item.id !== id)
}
function add(kind: Kind, notice: Notice) {
  const id = ++sequence
  notices.value.push({ ...notice, kind, id })
  if (notice.duration && notice.duration > 0) timers.set(id, setTimeout(() => remove(id), notice.duration))
}
provide(notificationKey, { success: notice => add('success', notice), error: notice => add('error', notice) })
onBeforeUnmount(() => { for (const timer of timers.values()) clearTimeout(timer) })
</script>

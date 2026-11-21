<script setup lang="ts">
import { inject, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { themeModeKey } from '../theme'
import { UiButton, UiModal, UiRadio, UiRadioGroup } from '../ui'
import UiIcon from './UiIcon.vue'

const { locale, t } = useI18n()
const dark = inject(themeModeKey, ref(false))
const open = ref(false)
const languageLabel = useId()
const themeLabel = useId()
</script>

<template>
  <UiButton quaternary size="small" aria-haspopup="dialog" :title="t('topbar.settings')"
    :aria-label="t('topbar.settings')" @click="open = true">
    <UiIcon name="gear" :size="18" />
  </UiButton>
  <UiModal v-model:show="open" :title="t('topbar.settings')">
    <div class="settings-fields">
      <span :id="languageLabel">{{ t('shell.language') }}</span>
      <UiRadioGroup v-model:value="locale" :aria-labelledby="languageLabel">
        <UiRadio value="ru"><span lang="ru">Русский</span></UiRadio>
        <UiRadio value="en"><span lang="en">English</span></UiRadio>
      </UiRadioGroup>
      <span :id="themeLabel">{{ t('topbar.theme') }}</span>
      <UiRadioGroup :value="dark ? 'dark' : 'light'" :aria-labelledby="themeLabel" @update:value="dark = $event === 'dark'">
        <UiRadio value="light">{{ t('topbar.light') }}</UiRadio>
        <UiRadio value="dark">{{ t('topbar.dark') }}</UiRadio>
      </UiRadioGroup>
    </div>
  </UiModal>
</template>

<style scoped>
.settings-fields { display: grid; gap: 8px; }
.settings-fields > span[id] { font-weight: 600; }
.settings-fields > span[id]:not(:first-child) { margin-top: 12px; }
</style>

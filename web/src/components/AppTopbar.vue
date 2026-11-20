<template>
  <div class="app-topbar">
    <span class="app-topbar__title">{{ t('app.title') }}</span>
    <span v-if="server" class="app-topbar__server">{{ server }}</span>
    <div class="app-topbar__actions">
      <n-button quaternary size="small" @click="toggleTheme">
        {{ t('topbar.theme') }}
      </n-button>
      <n-button
        quaternary
        size="small"
        :type="locale === 'ru' ? 'primary' : 'default'"
        @click="setLocale('ru')"
      >
        RU
      </n-button>
      <n-button
        quaternary
        size="small"
        :type="locale === 'en' ? 'primary' : 'default'"
        @click="setLocale('en')"
      >
        EN
      </n-button>
      <n-button size="small" @click="logout">{{ t('topbar.logout') }}</n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton } from 'naive-ui'

import { useSessionStore } from '../stores/session'
import { themeToggleKey } from '../theme'

const { t, locale } = useI18n()
const router = useRouter()
const store = useSessionStore()

const toggleTheme = inject(themeToggleKey, () => {})

// e.g. "mysql 8.4.0 · root"
const server = computed(() => {
  const driver = store.sessionInfo?.driver
  return driver ? `${driver.flavor} ${driver.version} · ${driver.user}` : ''
})

function setLocale(next: 'ru' | 'en'): void {
  locale.value = next
}

async function logout(): Promise<void> {
  try {
    await store.logout()
  } catch {
    // The store clears the local session even when the DELETE fails;
    // go to the login screen regardless.
  }
  await router.push('/login')
}
</script>

<style scoped>
.app-topbar {
  display: flex;
  align-items: center;
  gap: 16px;
  height: 48px;
  padding: 0 16px;
}

.app-topbar__title {
  font-weight: 600;
}

.app-topbar__server {
  opacity: 0.7;
}

.app-topbar__actions {
  display: flex;
  gap: 4px;
  margin-left: auto;
}
@media (max-width: 719px) {
  .app-topbar { height: auto; min-height: 44px; padding: 6px 10px; gap: 4px 10px; flex-wrap: wrap; }
  .app-topbar__server { flex: 1; min-width: 80px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .app-topbar__actions { flex-wrap: wrap; }
}
</style>

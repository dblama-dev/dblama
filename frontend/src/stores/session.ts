import { defineStore } from 'pinia'

import { useCatalogStore } from './catalog'
import { useMetadataStore } from './metadata'
import { api } from '../api/client'
import type { DriverMeta, LoginForm, SessionInfo } from '../api/types'

export const useSessionStore = defineStore('session', {
  state: () => ({
    sessionInfo: null as SessionInfo | null,
    drivers: [] as DriverMeta[],
    /** True once a GET /api/session probe has failed and no session
     * cookie can exist; the router guard skips further probes until the
     * next login or logout resets it. */
    authChecked: false,
  }),
  actions: {
    /** Login creates the session cookie (204, no body), then loads the
     * session info with a follow-up GET. */
    async login(form: LoginForm): Promise<void> {
      await api('POST', '/api/session', form)
      useCatalogStore().$reset()
      useMetadataStore().clear()
      this.authChecked = false
      await this.fetchSession()
    },
    /** Logout deletes the server session; local state is cleared even
     * when the DELETE fails, and the error propagates. */
    async logout(): Promise<void> {
      try {
        await api('DELETE', '/api/session')
      } finally {
        useCatalogStore().$reset()
        useMetadataStore().clear()
        this.sessionInfo = null
        this.authChecked = false
      }
    },
    async loadDrivers(): Promise<void> {
      this.drivers = await api<DriverMeta[]>('GET', '/api/drivers')
    },
    async fetchSession(): Promise<void> {
      this.sessionInfo = await api<SessionInfo>('GET', '/api/session')
    },
  },
})

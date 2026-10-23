import { defineStore } from 'pinia'

import { api } from '../api/client'
import type { DriverMeta, LoginForm, SessionInfo } from '../api/types'

export const useSessionStore = defineStore('session', {
  state: () => ({
    sessionInfo: null as SessionInfo | null,
    drivers: [] as DriverMeta[],
  }),
  actions: {
    /** Login creates the session cookie (204, no body), then loads the
     * session info with a follow-up GET. */
    async login(form: LoginForm): Promise<void> {
      await api('POST', '/api/session', form)
      await this.fetchSession()
    },
    /** Logout deletes the server session; local state is cleared even
     * when the DELETE fails, and the error propagates. */
    async logout(): Promise<void> {
      try {
        await api('DELETE', '/api/session')
      } finally {
        this.sessionInfo = null
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

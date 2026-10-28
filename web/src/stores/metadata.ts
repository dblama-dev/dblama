import { defineStore } from 'pinia'
import { api } from '../api/client'
import { useSessionStore } from './session'
export interface ServerMetadata {
  columnTypes: { name: string; category: string; hasLength?: boolean }[]
  collations: { name: string; charset?: string; database: boolean }[]
  operators: { value: string; label: string; arity: number }[]
}
export const useMetadataStore = defineStore('metadata', {
  state: () => ({ data: null as ServerMetadata | null, key: '', loading: false, generation: 0 }),
  actions: {
    clear(): void {
      this.generation++
      this.data = null
      this.key = ''
      this.loading = false
    },
    async load(): Promise<void> {
      const session = useSessionStore().sessionInfo
      if (!session) return
      const key = `${session.driver.flavor}|${session.driver.version}|${session.driver.host}|${session.driver.user}|${session.database}`
      if (this.key === key && (this.data || this.loading)) return
      const generation = ++this.generation
      this.key = key; this.loading = true; this.data = null
      try {
        const result = await api<ServerMetadata>('GET', '/api/metadata')
        if (this.generation === generation && Array.isArray(result.columnTypes) && Array.isArray(result.collations) && Array.isArray(result.operators)) this.data = result
      } catch { /* Keep editable fallback metadata when catalog privileges are limited. */ }
      finally { if (this.generation === generation) this.loading = false }
    },
  },
})

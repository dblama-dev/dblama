import { defineStore } from 'pinia'
import { watch } from 'vue'
import { api } from '../api/client'
import { withSchema } from '../lib/schema'
import { useCatalogStore } from './catalog'
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
    async load(db: string, schema: string | null): Promise<void> {
      const store = useSessionStore()
      const session = store.sessionInfo
      if (!session?.driver) return
      const { flavor, version, host, port, user } = session.driver
      const path = flavor === 'postgres'
        ? withSchema(`/api/databases/${encodeURIComponent(db)}/metadata`, schema, true)
        : '/api/metadata'
      const key = JSON.stringify([flavor, version, host, port, user, path])
      if (this.key === key && (this.data || this.loading)) return
      const generation = ++this.generation
      this.key = key; this.loading = true; this.data = null
      try {
        const result = await api<ServerMetadata>('GET', path)
        if (this.generation === generation && store.sessionInfo === session && Array.isArray(result.columnTypes) && Array.isArray(result.collations) && Array.isArray(result.operators)) this.data = result
      } catch { /* Keep editable fallback metadata when catalog privileges are limited. */ }
      finally { if (this.generation === generation) this.loading = false }
    },
  },
})

export function useDatabaseMetadata(db: () => string) {
  const metadata = useMetadataStore()
  const session = useSessionStore()
  const catalog = useCatalogStore()
  watch([db, () => catalog.schema, () => session.sessionInfo], ([database, schema, info], [, , priorInfo]) => {
    if (priorInfo !== undefined && info !== priorInfo) metadata.clear()
    if (database && info) void metadata.load(database, schema)
  }, { immediate: true })
  return metadata
}

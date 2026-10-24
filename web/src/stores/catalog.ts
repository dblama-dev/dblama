import { defineStore } from 'pinia'

import { api } from '../api/client'
import type { TableInfo } from '../api/types'

// Catalog of one database's tables. The store is a thin cache: after any
// DDL (PUT /tables/{table}, DELETE, truncate) a component calls
// invalidate(db) to refetch — the store does not watch mutations itself.
export const useCatalogStore = defineStore('catalog', {
  state: () => ({
    tables: [] as TableInfo[],
    loading: false,
    error: null as string | null,
  }),
  actions: {
    /** GET /api/databases/{db}/tables → {tables}. loading is true only
     * while the request is in flight; on failure error carries the
     * message and tables keep their previous value. */
    async fetchTables(db: string): Promise<void> {
      this.loading = true
      try {
        const body = await api<{ tables: TableInfo[] }>(
          'GET',
          `/api/databases/${encodeURIComponent(db)}/tables`,
        )
        this.tables = body.tables
        this.error = null
      } catch (err) {
        this.error = err instanceof Error ? err.message : String(err)
      } finally {
        this.loading = false
      }
    },
    /** Refetch after DDL. Same request as fetchTables; the distinct name
     * documents the intent at the call site. */
    async invalidate(db: string): Promise<void> {
      await this.fetchTables(db)
    },
  },
})

import { defineStore } from 'pinia'

import { api } from '../api/client'
import { withSchema } from '../lib/schema'
import type { TableInfo } from '../api/types'

// Catalog of one database's tables. The store is a thin cache: after any
// DDL (PUT /tables/{table}, DELETE, truncate) a component calls
// invalidate(db) to refetch — the store does not watch mutations itself.
export const useCatalogStore = defineStore('catalog', {
  state: () => ({
    tables: [] as TableInfo[],
    loading: false,
    error: null as string | null,
    requestID: 0,
    tablesPath: null as string | null,
    databaseRevision: 0,
    // Schema support (PostgreSQL): the current schema (null = driver
    // default "public") and whether the driver is schema-aware at all.
    schema: null as string | null,
    schemaAware: false,
  }),
  actions: {
    path(path: string): string {
      return withSchema(path, this.schema, this.schemaAware)
    },
    /** GET /api/databases/{db}/tables → {tables}. loading is true only
     * while the request is in flight; on failure error carries the
     * message and tables keep their previous value. */
    async fetchTables(db: string): Promise<void> {
      const requestID = ++this.requestID
      const path = this.path(`/api/databases/${encodeURIComponent(db)}/tables`)
      this.loading = true
      try {
        const body = await api<{ tables: TableInfo[] }>(
          'GET',
          path,
        )
        if (requestID !== this.requestID) return
        this.tables = body.tables
        this.tablesPath = path
        this.error = null
      } catch (err) {
        if (requestID !== this.requestID) return
        this.error = err instanceof Error ? err.message : String(err)
      } finally {
        if (requestID === this.requestID) this.loading = false
      }
    },
    /** Refetch after DDL. Same request as fetchTables; the distinct name
     * documents the intent at the call site. */
    async invalidate(db: string): Promise<void> {
      await this.fetchTables(db)
    },
    /** selectSchema makes schema current and refetches the table list.
     * Used only by schema-aware (PostgreSQL) connections. */
    async selectSchema(db: string, schema: string): Promise<void> {
      this.tables = []
      this.schema = schema
      this.schemaAware = true
      await this.fetchTables(db)
    },
    /** markSchemaAware records that the connected driver supports
     * schemas (caps.schemas); the tree shows the schema selector. */
    markSchemaAware(): void {
      this.schemaAware = true
    },
  },
})

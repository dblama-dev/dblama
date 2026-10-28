import { api } from '../api/client'

export interface DatabaseMetadata {
  columnTypes: { name: string; category: string; hasLength?: boolean }[]
  collations: { name: string; charset?: string; database: boolean }[]
  operators: { value: string; label: string; arity: number }[]
}
export function validDatabaseName(name: string): boolean {
  return name.trim().length > 0 && !name.includes('\0')
}
function requireName(name: string): void {
  if (!validDatabaseName(name)) throw new Error('A database name is required')
}
export async function createDatabase(name: string, collation = ''): Promise<void> {
  requireName(name)
  await api('POST', '/api/databases', { name, collation })
}
export async function renameDatabase(source: string, name: string): Promise<void> {
  requireName(source); requireName(name)
  if (source === name) throw new Error('Choose a different database name')
  await api('PATCH', `/api/databases/${encodeURIComponent(source)}`, { name })
}
export async function dropDatabase(name: string): Promise<void> {
  requireName(name)
  await api('DELETE', `/api/databases/${encodeURIComponent(name)}`)
}
export async function transferTables(source: string, tables: string[], targetDB: string, action: 'copy' | 'move'): Promise<void> {
  requireName(source); requireName(targetDB)
  if (source === targetDB || !tables.length || tables.some(t => !validDatabaseName(t)) || new Set(tables).size !== tables.length) {
    throw new Error('Select tables and a different target database')
  }
  await api('POST', `/api/databases/${encodeURIComponent(source)}/table-transfer`, { action, tables, targetDB })
}

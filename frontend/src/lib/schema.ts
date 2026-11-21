import { api } from '../api/client'
import type { SequenceInfo } from '../api/types'

export async function fetchSchemas(db: string): Promise<string[]> {
  return (await api<{ schemas: string[] }>('GET', `/api/databases/${encodeURIComponent(db)}/schemas`)).schemas
}
function sequencePath(db: string, schema: string, name?: string): string {
  const base = `/api/databases/${encodeURIComponent(db)}/sequences`
  return withSchema(name === undefined ? base : `${base}/${encodeURIComponent(name)}`, schema, true)
}
export async function fetchSequences(db: string, schema: string): Promise<SequenceInfo[]> {
  return (await api<{ sequences: SequenceInfo[] }>('GET', sequencePath(db, schema))).sequences
}
export function createSequence(db: string, name: string, increment: number, schema: string): Promise<unknown> {
  return api('POST', sequencePath(db, schema), { name, increment })
}
export function dropSequence(db: string, name: string, schema: string): Promise<unknown> {
  return api('DELETE', sequencePath(db, schema, name))
}
/** Append only after the entire endpoint path and existing query are built. */
export function withSchema(path: string, schema: string | null, aware: boolean): string {
  if (!aware || !schema) return path
  return `${path}${path.includes('?') ? '&' : '?'}schema=${encodeURIComponent(schema)}`
}

export async function alterSequence(db: string, name: string, changes: {restartWith?: number; increment?: number}, schema: string): Promise<void> {
  if (changes.increment === 0 || Object.values(changes).some(value => !Number.isSafeInteger(value))) throw new Error('Sequence values must be safe nonzero integers')
  const { restartWith, increment } = changes
  await api('PATCH', sequencePath(db, schema, name), { restart: restartWith, increment })
}

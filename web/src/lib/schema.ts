import { api } from '../api/client'
import type { SequenceInfo } from '../api/types'

export async function fetchSchemas(): Promise<string[]> {
  return (await api<{ schemas: string[] }>('GET', '/api/schemas')).schemas
}
export async function fetchSequences(schema: string): Promise<SequenceInfo[]> {
  return (await api<{ sequences: SequenceInfo[] }>('GET', withSchema('/api/sequences', schema, true))).sequences
}
export function createSequence(name: string, increment: number, schema: string): Promise<unknown> {
  return api('POST', withSchema('/api/sequences', schema, true), { name, increment })
}
export function dropSequence(name: string, schema: string): Promise<unknown> {
  return api('DELETE', withSchema(`/api/sequences/${encodeURIComponent(name)}`, schema, true))
}
/** Append only after the entire endpoint path and existing query are built. */
export function withSchema(path: string, schema: string | null, aware: boolean): string {
  if (!aware || !schema) return path
  return `${path}${path.includes('?') ? '&' : '?'}schema=${encodeURIComponent(schema)}`
}

export async function alterSequence(name: string, changes: {restartWith?: number; increment?: number}, schema: string): Promise<void> {
  if (changes.increment === 0 || Object.values(changes).some(value => !Number.isSafeInteger(value))) throw new Error('Sequence values must be safe nonzero integers')
  const { restartWith, increment } = changes
  await api('PATCH', withSchema(`/api/sequences/${encodeURIComponent(name)}`, schema, true), { restart: restartWith, increment })
}

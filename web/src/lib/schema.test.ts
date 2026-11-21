import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { alterSequence, createSequence, dropSequence, fetchSchemas, withSchema } from './schema'
vi.mock('../api/client', () => ({ api: vi.fn() }))
beforeEach(() => vi.clearAllMocks())
it('addresses sequence mutations with the explicitly selected schema', async () => {
  await createSequence('Odd Name', 2, 'Odd Schema')
  await dropSequence('Odd Name', 'Odd Schema')
  expect(api).toHaveBeenCalledWith('POST', '/api/sequences?schema=Odd%20Schema', { name: 'Odd Name', increment: 2 })
  expect(api).toHaveBeenCalledWith('DELETE', '/api/sequences/Odd%20Name?schema=Odd%20Schema')
})
it('does not turn a schema loading error into an empty successful list', async () => {
  vi.mocked(api).mockRejectedValueOnce(new Error('permission denied'))
  await expect(fetchSchemas()).rejects.toThrow('permission denied')
})
it('preserves query parameters and leaves MySQL paths unchanged', () => {
  expect(withSchema('/rows?limit=50', 'Odd Schema', true)).toBe('/rows?limit=50&schema=Odd%20Schema')
  expect(withSchema('/rows', 'public', false)).toBe('/rows')
})
it('alters a sequence in its explicit schema and rejects zero increment', async () => {
  await alterSequence('Odd Name', {restartWith: 20, increment:-2}, 'Odd Schema')
  expect(api).toHaveBeenCalledWith('PATCH','/api/sequences/Odd%20Name?schema=Odd%20Schema',{restart:20,increment:-2})
  await expect(alterSequence('seq',{increment:0},'public')).rejects.toThrow()
})

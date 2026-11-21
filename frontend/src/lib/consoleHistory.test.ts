import { beforeEach, describe, expect, it } from 'vitest'

import { loadHistory, pushHistory } from './consoleHistory'

// The storage key is a plain per-server suffix under dblama-console-.
const KEY = 'dblama-console-mysql|localhost'

describe('consoleHistory', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('returns an empty list when nothing is stored yet', () => {
    expect(loadHistory('mysql|localhost')).toEqual([])
  })

  it('persists pushed entries and orders them most-recent-first', () => {
    pushHistory('mysql|localhost', 'SELECT 1')
    pushHistory('mysql|localhost', 'SELECT 2')

    expect(loadHistory('mysql|localhost')).toEqual(['SELECT 2', 'SELECT 1'])

    // The list survives a "reload" because it lives in localStorage.
    expect(localStorage.getItem(KEY)).toBe(JSON.stringify(['SELECT 2', 'SELECT 1']))
  })

  it('deduplicates a consecutive repeat of the most recent entry', () => {
    pushHistory('mysql|localhost', 'SELECT 1')
    pushHistory('mysql|localhost', 'SELECT 1')

    expect(loadHistory('mysql|localhost')).toEqual(['SELECT 1'])
  })

  it('keeps a non-consecutive duplicate (only consecutive runs are merged)', () => {
    pushHistory('mysql|localhost', 'SELECT 1')
    pushHistory('mysql|localhost', 'SELECT 2')
    pushHistory('mysql|localhost', 'SELECT 1')

    expect(loadHistory('mysql|localhost')).toEqual(['SELECT 1', 'SELECT 2', 'SELECT 1'])
  })

  it('never stores whitespace-only sql', () => {
    pushHistory('mysql|localhost', '   ')
    pushHistory('mysql|localhost', '\n\t')

    expect(loadHistory('mysql|localhost')).toEqual([])
  })

  it('caps the stored history at 100 entries', () => {
    for (let i = 0; i < 105; i++) {
      pushHistory('mysql|localhost', `SELECT ${i}`)
    }

    const history = loadHistory('mysql|localhost')
    expect(history).toHaveLength(100)
    // Most recent first: the first entries are the newest pushes.
    expect(history[0]).toBe('SELECT 104')
    expect(history).not.toContain('SELECT 0')
  })

  it('treats a corrupted stored payload as an empty history', () => {
    localStorage.setItem(KEY, 'not json')
    expect(loadHistory('mysql|localhost')).toEqual([])

    // A push overwrites the corruption instead of extending it.
    pushHistory('mysql|localhost', 'SELECT 1')
    expect(loadHistory('mysql|localhost')).toEqual(['SELECT 1'])
  })

  it('separates histories of different servers', () => {
    pushHistory('mysql|localhost', 'SELECT 1')
    pushHistory('postgres|db2', 'SELECT 2')

    expect(loadHistory('mysql|localhost')).toEqual(['SELECT 1'])
    expect(loadHistory('postgres|db2')).toEqual(['SELECT 2'])
  })
})

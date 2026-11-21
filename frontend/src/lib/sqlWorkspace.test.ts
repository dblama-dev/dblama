import { beforeEach, expect, it, vi } from 'vitest'
import { loadBookmarks, loadDraft, removeBookmark, saveBookmark, saveDraft, sqlScopeKey, starterQuery } from './sqlWorkspace'

beforeEach(() => localStorage.clear())
it('isolates server port, account, database and schema without delimiter collisions', () => {
  const scope = { flavor:'mysql',host:'localhost',port:'3306',user:'root',database:'app',schema:'' }
  const key = sqlScopeKey(scope)
  saveDraft(key, 'SELECT draft')
  expect(loadDraft(key)).toBe('SELECT draft')
  for (const changed of [{port:'3307'}, {user:'reader'}, {database:'other'}, {schema:'private'}]) {
    expect(loadDraft(sqlScopeKey({...scope,...changed}))).toBeNull()
  }
  expect(sqlScopeKey({...scope,host:'a|b',user:'c'})).not.toBe(sqlScopeKey({...scope,host:'a',user:'b|c'}))
})
it('saves, replaces and deletes named bookmarks only within the current scope', () => {
  expect(saveBookmark('a','  Report  ','SELECT 1')).toBe(true)
  expect(saveBookmark('a','Report','SELECT 2')).toBe(true)
  expect(loadBookmarks('a')).toEqual([{name:'Report',sql:'SELECT 2'}])
  expect(loadBookmarks('b')).toEqual([])
  removeBookmark('a','Report')
  expect(loadBookmarks('a')).toEqual([])
  expect(saveBookmark('a',' ','SELECT 2')).toBe(false)
})
it('handles unavailable browser storage without crashing or reporting a saved bookmark', () => {
  const get = vi.spyOn(Storage.prototype,'getItem').mockImplementation(()=>{throw new Error('denied')})
  const set = vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('quota')})
  try {
    expect(loadDraft('a')).toBeNull()
    expect(loadBookmarks('a')).toEqual([])
    expect(saveDraft('a','SELECT 1')).toBe(false)
    expect(saveBookmark('a','report','SELECT 1')).toBe(false)
  } finally { get.mockRestore();set.mockRestore() }
})
it('quotes starter identifiers without executing or interpolating raw SQL', () => {
  expect(starterQuery('a`b','mysql')).toBe('SELECT * FROM `a``b` LIMIT 50;')
  expect(starterQuery('a"b','postgres','Odd Schema')).toBe('SELECT * FROM "Odd Schema"."a""b" LIMIT 50;')
})

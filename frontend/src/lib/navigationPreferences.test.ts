import {expect,it} from 'vitest'
import {readNavigationPreferences,writeNavigationPreferences,rememberTable,toggleFavorite} from './navigationPreferences'
it('falls back safely when storage is corrupt or unavailable',()=>{
 expect(readNavigationPreferences({getItem:()=>'{bad'})).toEqual({collapsed:false,width:220,recent:[],favorites:[]})
 expect(readNavigationPreferences({getItem:()=>{throw Error('denied')}}).width).toBe(220)
 expect(()=>writeNavigationPreferences({setItem:()=>{throw Error('quota')}},readNavigationPreferences())).not.toThrow()
})
it('bounds width and preserves scoped favorites and recent unique tables',()=>{
 const state=readNavigationPreferences({getItem:()=>JSON.stringify({width:999,collapsed:true,recent:[],favorites:[]})})
 expect(state.width).toBe(420)
 const table={db:'app',table:'x',schema:'odd',scope:'mysql:user@host'}
 expect(rememberTable(rememberTable(state,table),table).recent).toEqual([table])
 expect(toggleFavorite(toggleFavorite(state,table),table).favorites).toEqual([])
})

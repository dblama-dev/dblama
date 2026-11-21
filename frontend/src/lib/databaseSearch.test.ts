import { afterEach, describe, expect, it, vi } from 'vitest'
import { searchDatabase, searchResultLink } from './databaseSearch'

const definition = { columns: [{name:'body',type:'text',nullable:true},{name:'id',type:'bigint',nullable:false}], indexes:[], foreignKeys:[], options:{} }
const page = {columns:[{name:'body',type:'text',nullable:true}],rows:[['a literal match']],totalRows:1}
function response(body:unknown,status=200):Response { return {ok:status===200,status,json:async()=>body} as Response }
afterEach(()=>{vi.unstubAllGlobals();vi.useRealTimers()})

describe('bounded database search',()=>{
 it('encodes identifiers and schema and sends literal CONTAINS values with a 20-row limit',async()=>{
  const paths:string[]=[]
  vi.stubGlobal('fetch',vi.fn(async(path:string,init:RequestInit)=>{
   paths.push(path)
   expect(init.credentials).toBe('same-origin')
   expect(init.headers).toEqual({'X-Requested-With':'XMLHttpRequest'})
   expect(init.signal).toBeInstanceOf(AbortSignal)
   return response(path.includes('/rows?')?page:definition)
  }))
  const results:unknown[]=[]
  await searchDatabase({db:'app %/db',schema:'Odd /%',tables:['a %/table'],text:'50%_\\end',onResult:r=>results.push(r)})
  expect(paths[0]).toBe('/api/databases/app%20%25%2Fdb/tables/a%20%25%2Ftable?schema=Odd%20%2F%25')
  const rowsUrl=new URL(paths[1]!, 'https://local.test')
  expect(rowsUrl.searchParams.get('filter')).toBe('body:CONTAINS:50%_\\end')
  expect(rowsUrl.searchParams.get('limit')).toBe('20')
  expect(rowsUrl.searchParams.get('schema')).toBe('Odd /%')
  expect(results).toHaveLength(1)
 })
 it('rejects empty text and more than 20 tables before sending a request',async()=>{
  const fetch=vi.fn();vi.stubGlobal('fetch',fetch)
  await expect(searchDatabase({db:'app',tables:['a'],text:'',onResult:()=>{}})).rejects.toThrow('emptyQuery')
  await expect(searchDatabase({db:'app',tables:Array.from({length:21},(_,n)=>String(n)),text:'x',onResult:()=>{}})).rejects.toThrow('tooManyTables')
  expect(fetch).not.toHaveBeenCalled()
 })
 it('stops scheduling after cancellation, including in-flight metadata',async()=>{
  const abort=new AbortController();const paths:string[]=[]
  vi.stubGlobal('fetch',vi.fn(async(path:string)=>{paths.push(path);abort.abort();return response(definition)}))
  const result=await searchDatabase({db:'app',tables:['a','b','c'],text:'x',signal:abort.signal,onResult:()=>{throw Error('unexpected result')}})
  expect(result.cancelled).toBe(true)
  expect(paths.every(path=>!path.includes('/rows'))).toBe(true)
  expect(paths.length).toBeLessThanOrEqual(2)
 })
 it('reports an inaccessible table and continues searching others',async()=>{
  vi.stubGlobal('fetch',vi.fn(async(path:string)=>response(path.includes('/denied')?{error:{message:'permission denied'}}:path.includes('/rows?')?page:definition,path.includes('/denied')?403:200)))
  const results:unknown[]=[];const errors:unknown[]=[]
  const summary=await searchDatabase({db:'app',tables:['denied','allowed'],text:'match',onResult:r=>results.push(r),onError:e=>errors.push(e)})
  expect(errors).toEqual([{table:'denied',message:'permission denied'}])
  expect(results).toHaveLength(1)
  expect(summary.completedTables).toBe(2)
 })
 it('runs at most two table requests concurrently and only searches supported text columns',async()=>{
  let active=0,max=0;const filters:string[]=[]
  vi.stubGlobal('fetch',vi.fn(async(path:string)=>{
   active++;max=Math.max(max,active);await new Promise(resolve=>setTimeout(resolve,1));active--
   if(path.includes('/rows?')){filters.push(new URL(path,'https://local').searchParams.get('filter')!);return response(page)}
   return response({...definition,columns:[...definition.columns,{name:'tags',type:'text[]',nullable:true},{name:'doc',type:'json',nullable:true},{name:'title',type:'character varying',nullable:true}]})
  }))
  await searchDatabase({db:'app',tables:['a','b','c'],text:'x',onResult:()=>{}})
  expect(max).toBe(2)
  expect(filters).toEqual(expect.arrayContaining(['body:CONTAINS:x','title:CONTAINS:x']))
  expect(filters).toHaveLength(6)
 })
 it('aborts timed-out requests and reports the error without hanging',async()=>{
  vi.useFakeTimers()
  vi.stubGlobal('fetch',vi.fn((_path:string,init:RequestInit)=>new Promise((_resolve,reject)=>init.signal!.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError'))))))
  const errors:unknown[]=[]
  const pending=searchDatabase({db:'app',tables:['a'],text:'x',requestTimeoutMs:10,onResult:()=>{},onError:e=>errors.push(e)})
  await vi.advanceTimersByTimeAsync(10)
  const summary=await pending
  expect(summary.cancelled).toBe(false)
  expect(errors).toEqual([{table:'a',message:'requestTimeout'}])
 })
 it('builds links preserving encoded context and the exact literal row filter',()=>{
  const url=new URL(searchResultLink('app /%','t /%','body','a_b', 'Odd /%'),'https://local')
  expect(url.pathname).toBe('/db/app%20%2F%25/table/t%20%2F%25')
  expect(url.searchParams.get('schema')).toBe('Odd /%')
  expect(JSON.parse(url.searchParams.get('filters')!)).toEqual([{column:'body',op:'CONTAINS',value:'a_b'}])
  expect(url.searchParams.get('view')).toBe('data')
 })
})

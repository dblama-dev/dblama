import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { createI18n } from 'vue-i18n'
import { useCatalogStore } from '../stores/catalog'
import { searchWorkflowMessages } from '../i18n/searchWorkflows'
import SearchView from './SearchView.vue'
const routeState = reactive({ params: { db: 'app /%' }, query: { schema: 'Odd schema' } as Record<string,string> })
vi.mock('vue-router',()=>({useRoute:()=>routeState,useRouter:()=>({push:vi.fn()})}))
beforeEach(()=>{routeState.params.db='app /%';routeState.query={schema:'Odd schema'}})
afterEach(()=>vi.unstubAllGlobals())
function setup(){
 const pinia=createPinia();setActivePinia(pinia);const catalog=useCatalogStore();catalog.schema='Odd schema';catalog.schemaAware=true
 const i18n=createI18n({legacy:false,locale:'en',messages:searchWorkflowMessages})
 return mount(SearchView,{global:{plugins:[pinia,i18n]}})
}
function reply(body:unknown):Response{return {ok:true,status:200,json:async()=>body} as Response}
it('offers table scope, limits and matching row previews with a schema-aware filtered link',async()=>{
 vi.stubGlobal('fetch',vi.fn(async(path:string)=>reply(path.includes('/rows?')?{columns:[{name:'body',type:'text',nullable:true}],rows:[['a matched body']],totalRows:30}:path.includes('/tables?')?{tables:[{name:'a /%',rows:1,sizeBytes:0,engine:'InnoDB',comment:'',isView:false}]}:{columns:[{name:'body',type:'text',nullable:true}],indexes:[],foreignKeys:[],options:{}})))
 const wrapper=setup();await flushPromises()
 expect(wrapper.text()).toContain('20')
 await wrapper.get('input[type=checkbox]').setValue(true)
 await wrapper.get('input[data-search-text]').setValue('match_')
 await wrapper.get('form').trigger('submit');await flushPromises()
 expect(wrapper.text()).toContain('a matched body')
 expect(wrapper.text()).toContain('Search complete')
 const link=new URL(wrapper.get('a[data-search-result]').attributes('href')!,'https://local')
 expect(link.searchParams.get('schema')).toBe('Odd schema')
 expect(JSON.parse(link.searchParams.get('filters')!)).toEqual([{column:'body',op:'CONTAINS',value:'match_'}])
 wrapper.unmount()
})
it('cancels active requests and shows cancellation instead of success',async()=>{
 vi.stubGlobal('fetch',vi.fn(async(path:string,init:RequestInit)=>{
  if(path.includes('/tables?'))return reply({tables:[{name:'a',rows:0,sizeBytes:0,engine:'',comment:'',isView:false}]})
  return new Promise<Response>((_resolve,reject)=>init.signal!.addEventListener('abort',()=>reject(new DOMException('Stopped','AbortError'))))
 }))
 const wrapper=setup();await flushPromises();await wrapper.get('input[type=checkbox]').setValue(true);await wrapper.get('input[data-search-text]').setValue('a');await wrapper.get('form').trigger('submit');await flushPromises()
 await wrapper.get('[data-search-stop]').trigger('click');await flushPromises()
 expect(wrapper.text()).toContain('Search cancelled')
 expect(wrapper.text()).not.toContain('Search complete')
 wrapper.unmount()
})

const tableInfo = (name:string) => ({name,rows:0,sizeBytes:0,engine:'InnoDB',comment:'',isView:false})
it.each([['a /%',true],['missing table',false]])('preselects only an available exact table context: %s',async(table,checked)=>{
 routeState.query.table=table
 const paths:string[]=[]
 vi.stubGlobal('fetch',vi.fn(async(path:string)=>{paths.push(path);return reply({tables:[tableInfo('a /%')]})}))
 const wrapper=setup();await flushPromises()
 expect((wrapper.get('input[type=checkbox]').element as HTMLInputElement).checked).toBe(checked)
 expect(paths).toEqual(['/api/databases/app%20%2F%25/tables?schema=Odd%20schema'])
 if(checked){
  await wrapper.get('input[type=checkbox]').setValue(false)
  await useCatalogStore().fetchTables('app /%');await flushPromises()
  expect((wrapper.get('input[type=checkbox]').element as HTMLInputElement).checked).toBe(false)
 }
 wrapper.unmount()
})
it('ignores a stale scope response after a newer context loads and the user changes selection',async()=>{
 routeState.query.table='first'
 let resolveFirst!:(response:Response)=>void
 vi.stubGlobal('fetch',vi.fn((path:string)=>path.includes('/app%20%2F%25/')?new Promise<Response>(resolve=>{resolveFirst=resolve}):Promise.resolve(reply({tables:[tableInfo('second'),tableInfo('manual'),tableInfo('first')]}))))
 const wrapper=setup()
 routeState.params.db='new db';routeState.query.table='second'
 await flushPromises()
 const boxes=wrapper.findAll('input[type=checkbox]')
 expect((boxes[0]!.element as HTMLInputElement).checked).toBe(true)
 await boxes[0]!.setValue(false);await boxes[1]!.setValue(true)
 resolveFirst(reply({tables:[tableInfo('first')]}));await flushPromises()
 expect((boxes[0]!.element as HTMLInputElement).checked).toBe(false)
 expect((boxes[1]!.element as HTMLInputElement).checked).toBe(true)
 expect((boxes[2]!.element as HTMLInputElement).checked).toBe(false)
 wrapper.unmount()
})

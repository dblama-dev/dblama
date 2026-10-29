import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { useCatalogStore } from '../stores/catalog'
import { searchWorkflowMessages } from '../i18n/searchWorkflows'
import SearchView from './SearchView.vue'
vi.mock('vue-router',()=>({useRoute:()=>({params:{db:'app /%'},query:{schema:'Odd schema'}}),useRouter:()=>({push:vi.fn()})}))
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

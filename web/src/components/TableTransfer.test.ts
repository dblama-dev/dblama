import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NButton, NSelect } from 'naive-ui'
import { api } from '../api/client'
import { useSessionStore } from '../stores/session'
import TableTransfer from './TableTransfer.vue'
import { databaseMessages } from '../i18n/databases'
import type { SessionInfo } from '../api/types'
vi.mock('../api/client', () => ({ api: vi.fn() }))
const request = vi.mocked(api)
async function setup(enabled = true) {
 const pinia=createPinia();setActivePinia(pinia)
 useSessionStore().sessionInfo={ caps: {moveCopy:enabled} } as SessionInfo
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/',component:{template:'<div />'}},{path:'/db/:db',name:'db',component:{template:'<div />'}}]});await router.push('/')
 return mount(TableTransfer,{props:{db:'source',tables:['items']},global:{plugins:[pinia,router,createI18n({legacy:false,locale:'en',messages:databaseMessages})]}})
}
beforeEach(()=>{request.mockReset();request.mockImplementation(async(method,path)=>{if(method==='GET'&&path==='/api/databases')return {databases:[{name:'source'},{name:'target'}]};return undefined})})
it('hides cross-database transfer when the driver does not support it',async()=>{const w=await setup(false);expect(w.findAllComponents(NButton)).toHaveLength(0);expect(request).not.toHaveBeenCalled()})
it('offers other databases and copies the selected tables before reporting changed',async()=>{
 const w=await setup();const click=async(text:string)=>{const b=w.findAllComponents(NButton).find(b=>b.text()===text);if(!b)throw Error(text);await b.trigger('click');await flushPromises()}
 await click('Copy tables')
 expect(w.findComponent(NSelect).props('options')).toEqual([{label:'target',value:'target'}])
 w.findComponent(NSelect).vm.$emit('update:value','target');await flushPromises();await w.find('form').trigger('submit');await flushPromises()
 expect(request).toHaveBeenCalledWith('POST','/api/databases/source/table-transfer',{action:'copy',tables:['items'],targetDB:'target'})
 expect(w.emitted('changed')).toHaveLength(1);expect(w.text()).toContain('Open target database')
})

it('keeps the destination link when the catalog clears the selection after transfer', async () => {
 const w = await setup()
 const button = (text:string) => w.findAllComponents(NButton).find(b => b.text() === text)
 await button('Copy tables')!.trigger('click'); await flushPromises()
 w.getComponent(NSelect).vm.$emit('update:value','target'); await flushPromises()
 await w.get('form').trigger('submit'); await flushPromises()
 await w.setProps({tables:[]})
 expect(button('Copy tables')).toBeUndefined()
 expect(button('Move tables')).toBeUndefined()
 expect(button('Open target database')).toBeDefined()
 w.unmount()
})

it('keeps the submitted transfer destination if selection changes during the request', async () => {
 const w = await setup()
 const copy = w.findAllComponents(NButton).find(b => b.text() === 'Copy tables')!
 await copy.trigger('click'); await flushPromises()
 w.getComponent(NSelect).vm.$emit('update:value','target'); await flushPromises()
 let finish!: (value: unknown) => void
 request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
 await w.get('form').trigger('submit')
 await w.setProps({tables:[]})
 finish(undefined); await flushPromises()
 expect(w.text()).toContain('Open target database')
 w.unmount()
})

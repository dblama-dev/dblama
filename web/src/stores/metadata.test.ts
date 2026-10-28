import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { useSessionStore } from './session'
import { useMetadataStore } from './metadata'
vi.mock('../api/client',()=>({api:vi.fn()}))
it('discards catalog responses from a session cleared during logout/reconnect',async()=>{
 setActivePinia(createPinia())
 const session=useSessionStore()
 session.sessionInfo={driver:{name:'test',flavor:'postgres',version:'17',host:'localhost',user:'u'},caps:{} as never,database:'app'}
 let finish!:(value:unknown)=>void
 vi.mocked(api).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 const metadata=useMetadataStore()
 const pending=metadata.load()
 // clear() is explicit, unlike Pinia $reset, so pending requests retain an invalidated generation.
 metadata.clear()
 vi.mocked(api).mockResolvedValueOnce({columnTypes:[{name:'fresh',category:'text'}],operators:[],collations:[]})
 await metadata.load()
 finish({columnTypes:[{name:'stale',category:'text'}],operators:[],collations:[]})
 await pending
 expect(metadata.data?.columnTypes[0]?.name).toBe('fresh')
})

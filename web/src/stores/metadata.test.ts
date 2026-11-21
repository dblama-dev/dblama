import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { useSessionStore } from './session'
import { useMetadataStore } from './metadata'
vi.mock('../api/client',()=>({api:vi.fn()}))
beforeEach(() => vi.mocked(api).mockReset())
it('discards catalog responses from a session cleared during logout/reconnect',async()=>{
 setActivePinia(createPinia())
 const session=useSessionStore()
 session.sessionInfo={driver:{name:'test',flavor:'postgres',version:'17',host:'localhost',user:'u'},caps:{} as never,database:'app'}
 let finish!:(value:unknown)=>void
 vi.mocked(api).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 const metadata=useMetadataStore()
 const pending=metadata.load('app', 'public')
 // clear() is explicit, unlike Pinia $reset, so pending requests retain an invalidated generation.
 metadata.clear()
 vi.mocked(api).mockResolvedValueOnce({columnTypes:[{name:'fresh',category:'text'}],operators:[],collations:[]})
 await metadata.load('app', 'public')
 finish({columnTypes:[{name:'stale',category:'text'}],operators:[],collations:[]})
 await pending
 expect(metadata.data?.columnTypes[0]?.name).toBe('fresh')
})

it.each([
  ['other db', 'public', '/api/databases/other%20db/metadata?schema=public'],
  ['app', 'Other Schema', '/api/databases/app/metadata?schema=Other%20Schema'],
])('uses metadata for selected database %s and schema %s after a scope change', async (db, schema, path) => {
  setActivePinia(createPinia())
  useSessionStore().sessionInfo = {
    driver: { name: 'PostgreSQL', flavor: 'postgres', version: '17', host: 'localhost', user: 'u' },
    caps: {} as never,
    database: null,
  }
  let finish!: (value: unknown) => void
  vi.mocked(api)
    .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    .mockResolvedValueOnce({ columnTypes: [{ name: 'current', category: 'text' }], operators: [], collations: [] })

  const metadata = useMetadataStore()
  const pending = metadata.load('app', 'public')
  await metadata.load(db, schema)
  expect(api).toHaveBeenLastCalledWith('GET', path)

  finish({ columnTypes: [{ name: 'stale', category: 'text' }], operators: [], collations: [] })
  await pending
  expect(metadata.data?.columnTypes[0]?.name).toBe('current')
})

it('shares server metadata between selected MySQL databases', async () => {
  setActivePinia(createPinia())
  useSessionStore().sessionInfo = {
    driver: { name: 'MySQL', flavor: 'mysql', version: '8.0', host: 'localhost', user: 'u' },
    caps: {} as never,
    database: null,
  }
  vi.mocked(api).mockResolvedValueOnce({ columnTypes: [], operators: [], collations: [] })
  const metadata = useMetadataStore()
  await metadata.load('app', null)
  await metadata.load('other db', null)
  expect(vi.mocked(api).mock.calls).toEqual([['GET', '/api/metadata']])
})

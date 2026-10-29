import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { downloadExport } from './download'
import type { ExportRequest } from '../api/types'
let fetcher: ReturnType<typeof vi.fn>
beforeEach(() => {
 fetcher=vi.fn().mockResolvedValue({ok:true,headers:new Headers(),blob:async()=>new Blob(['CSV'])})
 vi.stubGlobal('fetch',fetcher)
 URL.createObjectURL=vi.fn(()=> 'blob:download');URL.revokeObjectURL=vi.fn()
 vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(()=>{})
})
afterEach(()=>{vi.unstubAllGlobals();vi.restoreAllMocks()})
it.each(['csv','csv_semicolon','tsv'])('requests reversible CSV encoding for browser %s exports',async format=>{
 await downloadExport('db',{format,tables:['t']})
 expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({format,tables:['t'],csvEncoding:'escaped'})
})
it('preserves explicit legacy encoding and SQL requests',async()=>{
 const legacy={format:'csv',tables:['t'],csvEncoding:'legacy'} as ExportRequest
 await downloadExport('db',legacy)
 await downloadExport('db',{format:'sql',tables:[]})
 expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual(legacy)
 expect(JSON.parse(String(fetcher.mock.calls[1]?.[1]?.body))).toEqual({format:'sql',tables:[]})
})

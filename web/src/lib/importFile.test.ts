import { afterEach, expect, it, vi } from 'vitest'
import { runImport } from './importFile'
afterEach(() => vi.unstubAllGlobals())
it('rejects EOF without terminal success even after progress', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"done":100,"total":0}\n')))
 await expect(runImport('db', new FormData(), () => {})).rejects.toThrow(/complete|success|ended/i)
})
it('passes cancellation signal and preserves AbortError', async () => {
 const controller = new AbortController(); controller.abort()
 const fetcher = vi.fn().mockRejectedValue(new DOMException('Cancelled', 'AbortError')); vi.stubGlobal('fetch', fetcher)
 await expect(runImport('db', new FormData(), () => {}, null, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
 expect(fetcher.mock.calls[0]?.[1]?.signal).toBe(controller.signal)
})
it('reads a terminal record without a trailing newline', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"done":3,"total":3,"ok":true}')))
 await expect(runImport('db', new FormData(), () => {})).resolves.toBeUndefined()
})
it('rejects cancellation while reading and releases the response reader', async () => {
 const controller = new AbortController()
 const cancel = vi.fn().mockResolvedValue(undefined)
 const releaseLock = vi.fn()
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok:true,status:200,body:{getReader:()=>({
   read: async () => { controller.abort(); return {done:false,value:new TextEncoder().encode('{"ok":true}\n')} }, cancel, releaseLock,
 })}}))
 await expect(runImport('db',new FormData(),()=>{},null,controller.signal)).rejects.toMatchObject({name:'AbortError'})
 expect(cancel).toHaveBeenCalledOnce()
 expect(releaseLock).toHaveBeenCalledOnce()
})

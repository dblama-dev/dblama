import { expect, it } from 'vitest'
import { parseCsvPreview } from './csvPreview'
it('parses quoted delimiters, escaped quotes, multiline Unicode and empty values', () => {
 expect(parseCsvPreview('id,name,empty\r\n1,"Привет,\n""мир""",\r\n').rows).toEqual([['id','name','empty'],['1','Привет,\n"мир"','']])
})
it('rejects malformed quoting and inconsistent row widths', () => {
 expect(() => parseCsvPreview('a,b\n1,"unterminated')).toThrow(/quote/i)
 expect(() => parseCsvPreview('a,b\n1,2,3')).toThrow(/fields/i)
 expect(() => parseCsvPreview('a,b\n1,"closed"oops')).toThrow(/quote/i)
})
it('limits preview records and ignores an incomplete bounded tail', () => {
 expect(parseCsvPreview('a;b\n1;"cut', ';', '"', true).rows).toEqual([['a','b']])
 const result = parseCsvPreview(Array.from({length:100}, (_,i)=>`${i},value`).join('\n'))
 expect(result.rows.length).toBeLessThanOrEqual(21)
 expect(result.truncated).toBe(true)
})
it('bounds unusually wide records before rendering mapping controls', () => {
 expect(() => parseCsvPreview(Array.from({length:201},()=>'field').join(','))).toThrow(/columns|fields|wide/i)
})
it('supports MySQL single-rune separators, including pipe and Unicode', () => {
 expect(parseCsvPreview('id|value\n1|"pipe|value"\n','|').rows).toEqual([['id','value'],['1','pipe|value']])
 expect(parseCsvPreview('id🦄value\n1🦄Привет\n','🦄').rows).toEqual([['id','value'],['1','Привет']])
 expect(parseCsvPreview('a,b\n1,2\n','').rows).toEqual([['a','b'],['1','2']])
 for (const separator of ['||','\n','\r','"','\0']) expect(()=>parseCsvPreview('input',separator)).toThrow(/separator/i)
})

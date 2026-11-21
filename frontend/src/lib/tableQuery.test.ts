import { describe, expect, it } from 'vitest'

import { buildRowsQuery, ROW_FILTER_OPERATORS } from './tableQuery'

describe('buildRowsQuery', () => {
  it('builds limit/offset/order/filter params from the state', () => {
    const qs = buildRowsQuery({
      limit: 50,
      offset: 100,
      sort: { column: 'name', desc: false },
      filters: [{ column: 'age', op: '=', value: '30' }],
    })

    const params = new URLSearchParams(qs)
    expect(params.get('limit')).toBe('50')
    expect(params.get('offset')).toBe('100')
    expect(params.get('order')).toBe('name:asc')
    expect(params.getAll('filter')).toEqual(['age:=:30'])
  })

  it('emits desc for a descending sort', () => {
    const qs = buildRowsQuery({
      limit: 25,
      offset: 0,
      sort: { column: 'created_at', desc: true },
      filters: [],
    })

    expect(new URLSearchParams(qs).get('order')).toBe('created_at:desc')
  })

  it('omits the order param when sort is null', () => {
    const qs = buildRowsQuery({ limit: 50, offset: 0, sort: null, filters: [] })

    expect(qs).toBe('limit=50&offset=0')
  })

  it('sends IS NULL with a trailing colon and no value', () => {
    const qs = buildRowsQuery({
      limit: 50,
      offset: 0,
      sort: null,
      filters: [{ column: 'deleted_at', op: 'IS NULL', value: '' }],
    })

    const params = new URLSearchParams(qs)
    expect(params.getAll('filter')).toEqual(['deleted_at:IS NULL:'])
  })

  it('sends IS NOT NULL with a trailing colon even when a value is set', () => {
    const qs = buildRowsQuery({
      limit: 50,
      offset: 0,
      sort: null,
      filters: [{ column: 'x', op: 'IS NOT NULL', value: 'ignored' }],
    })

    const params = new URLSearchParams(qs)
    expect(params.getAll('filter')).toEqual(['x:IS NOT NULL:'])
  })

  it('preserves the order of multiple filters', () => {
    const qs = buildRowsQuery({
      limit: 50,
      offset: 0,
      sort: null,
      filters: [
        { column: 'a', op: 'LIKE', value: '%x%' },
        { column: 'b', op: '>=', value: '10' },
        { column: 'c', op: 'NOT LIKE', value: 'y' },
      ],
    })

    const params = new URLSearchParams(qs)
    expect(params.getAll('filter')).toEqual(['a:LIKE:%x%', 'b:>=:10', 'c:NOT LIKE:y'])
  })

  it('URL-encodes special characters so the backend sees the decoded col:op:value', () => {
    // URLSearchParams percent-encodes ':' and turns spaces into '+'; the Go
    // side decodes the query string before splitting, so the decoded
    // roundtrip must equal the raw col:op:value string.
    const qs = buildRowsQuery({
      limit: 50,
      offset: 0,
      sort: null,
      filters: [{ column: 'na me', op: '=', value: 'a:b&c' }],
    })

    expect(qs).toContain('filter=na+me%3A%3D%3Aa%3Ab%26c')
    const params = new URLSearchParams(qs)
    expect(params.getAll('filter')).toEqual(['na me:=:a:b&c'])
  })

  it('keeps colons inside a filter value intact (backend splits first two only)', () => {
    const qs = buildRowsQuery({
      limit: 50,
      offset: 0,
      sort: null,
      filters: [{ column: 'ts', op: '>', value: '2026-01-01 10:00:00' }],
    })

    const params = new URLSearchParams(qs)
    expect(params.getAll('filter')).toEqual(['ts:>:2026-01-01 10:00:00'])
  })

  it('defaults to limit 50 and offset 0 for an empty state', () => {
    const qs = buildRowsQuery({ limit: 50, offset: 0, sort: null, filters: [] })

    expect(qs).toBe('limit=50&offset=0')
  })

  it('exposes the backend operator whitelist in a stable order', () => {
    expect(ROW_FILTER_OPERATORS).toEqual([
      '=',
      '!=',
      '<',
      '>',
      '<=',
      '>=',
      'LIKE',
      'CONTAINS',
      'NOT LIKE',
      'REGEXP',
      'NOT REGEXP',
      'IN',
      'NOT IN',
      'IS NULL',
      'IS NOT NULL',
    ])
  })
})

import { readTableRouteQuery, writeTableRouteQuery, tableContextLink } from './tableQuery'
it('restores route grid state and ignores corrupt state', () => {
 expect(readTableRouteQuery({ page:'3',pageSize:'25',filters:JSON.stringify([{column:'name',op:'LIKE',value:'%a%'}]),sort:JSON.stringify({column:'id',desc:true}) })).toEqual({page:3,pageSize:25,filters:[{column:'name',op:'LIKE',value:'%a%'}],sort:{column:'id',desc:true}})
 expect(readTableRouteQuery({page:'-1',pageSize:'nan',filters:'no',sort:'{}'})).toEqual({page:1,pageSize:50,filters:[],sort:null})
})
it('writes query state without dropping schema and section',()=>{
 expect(writeTableRouteQuery({schema:'Odd',view:'data',extra:'yes'}, {page:2,pageSize:50,filters:[],sort:null})).toEqual({schema:'Odd',view:'data',extra:'yes',page:'2',pageSize:'50'})
 expect(tableContextLink('a/b','weird %','export','Odd Schema')).toBe('/db/a%2Fb/export?table=weird+%25&schema=Odd+Schema')
})
it('restores literal CONTAINS filters from database search links',()=>{
 expect(readTableRouteQuery({filters:JSON.stringify([{column:'name',op:'CONTAINS',value:'100%_!'}])}).filters).toEqual([{column:'name',op:'CONTAINS',value:'100%_!'}])
})

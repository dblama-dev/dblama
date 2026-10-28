import { expect, it } from 'vitest'
import { formatSQL } from './formatSQL'
it('formats queries in the selected dialect while preserving literals', () => {
 const pg = formatSQL("select 'FROM test' as text, 1::integer from test where id=2", 'postgres')
 expect(pg).toContain("'FROM test'")
 expect(pg).toContain('\nFROM')
 expect(pg).toContain('::integer')
 const mysql = formatSQL('select `odd name` from `test` where id=2', 'mysql')
 expect(mysql).toContain('`odd name`')
 expect(mysql).toContain('\nWHERE')
})

import { formatDialect, mariadb, mysql, postgresql } from 'sql-formatter'
export function formatSQL(source: string, flavor: string): string {
  return formatDialect(source, { dialect: flavor === 'postgres' ? postgresql : flavor === 'maria' ? mariadb : mysql, keywordCase: 'upper' })
}

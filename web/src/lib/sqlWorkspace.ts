// Bounded, browser-local SQL workspaces. These contain SQL, never credentials.
const KEY = 'dbm-sql-workspaces-v1'
const MAX_SQL = 1_000_000
interface Workspace { scope: string; draft?: string; bookmarks: Bookmark[] }
export interface Bookmark { name: string; sql: string }
export interface SQLScope { flavor: string; host: string; port?: string; user: string; database: string; schema?: string | null }
export function sqlScopeKey(scope: SQLScope): string {
  return JSON.stringify([scope.flavor,scope.host,scope.port ?? '',scope.user,scope.database,scope.schema ?? ''])
}
function read(): Workspace[] {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(KEY) ?? '[]')
    if (!Array.isArray(raw)) return []
    return raw.filter((v): v is Workspace => v && typeof v.scope === 'string' && Array.isArray(v.bookmarks))
      .slice(0,30).map(v => ({scope:v.scope,
        ...(typeof v.draft === 'string' && v.draft.length <= MAX_SQL ? {draft:v.draft} : {}),
        bookmarks:v.bookmarks.filter(b => b && typeof b.name === 'string' && typeof b.sql === 'string' && b.sql.length <= MAX_SQL).slice(0,50),
      }))
  } catch { return [] }
}
function update(scope: string, change: (entry: Workspace) => void): boolean {
  const list = read()
  const entry = list.find(v => v.scope === scope) ?? {scope,bookmarks:[]}
  change(entry)
  try { localStorage.setItem(KEY,JSON.stringify([entry,...list.filter(v => v.scope !== scope)].slice(0,30))); return true }
  catch { return false }
}
export function loadDraft(scope: string): string | null { return read().find(v => v.scope === scope)?.draft ?? null }
export function saveDraft(scope: string, sql: string): boolean {
  return sql.length <= MAX_SQL && update(scope,v => {v.draft = sql})
}
export function loadBookmarks(scope: string): Bookmark[] { return read().find(v => v.scope === scope)?.bookmarks ?? [] }
export function saveBookmark(scope: string, name: string, sql: string): boolean {
  name = name.trim()
  if (!name || name.length > 100 || !sql.trim() || sql.length > MAX_SQL) return false
  return update(scope,v => {v.bookmarks = [{name,sql},...v.bookmarks.filter(b => b.name !== name)].slice(0,50)})
}
export function removeBookmark(scope: string, name: string): boolean {
  return update(scope,v => {v.bookmarks = v.bookmarks.filter(b => b.name !== name)})
}
export function starterQuery(table: string, flavor: string, schema?: string | null): string {
  const quote = flavor === 'postgres' ? '"' : '`'
  const ident = (value: string) => quote + value.replaceAll(quote,quote + quote) + quote
  return `SELECT * FROM ${flavor === 'postgres' && schema ? ident(schema) + '.' : ''}${ident(table)} LIMIT 50;`
}

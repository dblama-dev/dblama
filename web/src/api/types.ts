// Mirror of the Go JSON contracts from server/internal/driver and
// server/internal/api: field names follow the Go json tags.

export interface DriverMeta {
  id: string
  name: string
  defaultPort: string
}

export interface DriverInfo {
  name: string
  flavor: string
  version: string
  user: string
  host: string
}

export interface Caps {
  views: boolean
  triggers: boolean
  routines: boolean
  procedures: boolean
  events: boolean
  partitions: boolean
  sequences: boolean
  schemas: boolean
  users: boolean
  monitor: boolean
  moveCopy: boolean
  comments: boolean
  upsert: boolean
  descendingIndexes: boolean
}

export interface DatabaseInfo {
  name: string
  sizeBytes: number
  collation: string
}

export interface SessionInfo {
  driver: DriverInfo
  caps: Caps
  database: string | null
}

export interface LoginForm {
  driver: string
  host: string
  port: string
  user: string
  password: string
  database?: string
}

// --- Tables and rows (mirrors of the Go types in server/internal/driver,
// camelCase as written in the json tags) ---

export interface TableInfo {
  name: string
  rows: number
  sizeBytes: number
  engine: string
  comment: string
  isView: boolean
}

export interface DefaultValue {
  value: string
  isSqlExpression?: boolean
}

export interface ColumnDef {
  name: string
  oldName?: string
  type: string
  length?: string
  unsigned?: boolean
  nullable: boolean
  default?: DefaultValue | null
  autoIncrement?: boolean
  onUpdate?: string
  generated?: string
  collation?: string
  comment?: string
  enumValues?: string[]
  position?: string
}

export interface IndexColumn {
  name: string
  subPart?: number
}

export interface Index {
  name: string
  type: string
  columns: IndexColumn[]
}

export interface ForeignKey {
  name: string
  columns: string[]
  refTable: string
  refColumns: string[]
  onDelete?: string
  onUpdate?: string
}

export interface TableOptions {
  engine?: string
  collation?: string
  comment?: string
  autoIncrement?: number
}

export interface TableDef {
  newName?: string
  columns: ColumnDef[]
  indexes: Index[]
  foreignKeys: ForeignKey[]
  options: TableOptions
}

export interface ColumnMeta {
  name: string
  type: string
  nullable: boolean
}

// One statement's outcome inside a console batch; mirrors the Go
// driver.Result json tags. A row-returning statement fills columns/rows,
// a non-row statement fills affectedRows/lastInsertId.
export interface QueryResult {
  columns: ColumnMeta[]
  rows: unknown[][]
  affectedRows?: number
  lastInsertId?: number
  truncated?: boolean
  seconds: number
}

export interface Page {
  columns: ColumnMeta[]
  rows: unknown[][]
  totalRows: number
}

// Mirror of driver.ExportRequest: format is "sql", "csv", "csv_semicolon"
// or "tsv". For sql an empty tables list dumps the whole database; the
// delimited formats require exactly one table (validated server-side and
// pre-checked in the UI). dropIfExists/ifNotExists only apply to sql,
// gzip wraps any format.
export interface ExportRequest {
  format: string
  tables: string[]
  dropIfExists?: boolean
  ifNotExists?: boolean
  gzip?: boolean
}

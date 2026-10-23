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

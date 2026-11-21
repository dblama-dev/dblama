import type { Caps } from '../api/types'

export interface Destination {
  label: string
  icon: string
  path: string
}

export const databaseTools = { console: 'terminal', search: 'search', import: 'upload', export: 'download' } as const

export const objectIcons = { views: 'eye', triggers: 'activity', routines: 'code', events: 'history', partitions: 'schema' } as const

export function databaseDestinations(db: string, caps?: Caps): Destination[] {
  const base = '/db/' + encodeURIComponent(db)
  const result = [
    {label: 'databaseOps.databaseActions', icon: 'settings', path: base + '/operations'},
    {label: 'relationships.title', icon: 'network', path: base + '/relationships'},
  ]
  for (const kind of Object.keys(objectIcons) as (keyof typeof objectIcons)[]) {
    if (caps?.[kind]) result.push({label: 'objects.' + kind, icon: objectIcons[kind], path: base + '/objects/' + kind})
  }
  return result
}

export function serverDestinations(caps?: Caps): Destination[] {
  const result: Destination[] = []
  if (caps?.users) result.push({label: 'admin.users', icon: 'user', path: '/server/users'})
  if (caps?.monitor) result.push({label: 'admin.monitor', icon: 'activity', path: '/server/processes'})
  return result
}

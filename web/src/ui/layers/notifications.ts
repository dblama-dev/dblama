import { inject, type InjectionKey } from 'vue'

export interface Notice {
  title?: string
  content: string
  duration?: number
}
export interface Notifications {
  success(notice: Notice): void
  error(notice: Notice): void
}

export const notificationKey: InjectionKey<Notifications> = Symbol('workspace-notifications')
export function useNotification(): Notifications {
  const service = inject(notificationKey)
  if (!service) throw new Error('Mount UiNotificationProvider to display notifications')
  return service
}

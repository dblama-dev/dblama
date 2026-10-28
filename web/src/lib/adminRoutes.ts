import type { RouteRecordRaw } from 'vue-router'
import BasicLayout from '../layouts/BasicLayout.vue'
import UsersView from '../views/UsersView.vue'
import MonitorView from '../views/MonitorView.vue'

export const adminRoutes: RouteRecordRaw[] = [
  { path: '/server/users', component: BasicLayout, children: [{ path: '', name: 'server-users', component: UsersView, meta: { capability: 'users' } }] },
  { path: '/server/processes', component: BasicLayout, children: [{ path: '', name: 'server-processes', component: MonitorView, meta: { capability: 'monitor' } }] },
]

import { createRouter, createWebHistory } from 'vue-router'

import { adminRoutes } from '../lib/adminRoutes'
import { objectRoutes } from '../lib/objectRoutes'
import BasicLayout from '../layouts/BasicLayout.vue'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import ConsoleView from '../views/ConsoleView.vue'
import DbView from '../views/DbView.vue'
import ExportView from '../views/ExportView.vue'
import ImportView from '../views/ImportView.vue'
import LoginView from '../views/LoginView.vue'
import ServerView from '../views/ServerView.vue'
import TableView from '../views/TableView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    ...adminRoutes,
    ...objectRoutes,
    { path: '/', redirect: '/server' },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/server',
      component: BasicLayout,
      children: [{ path: '', name: 'server', component: ServerView }],
    },
    {
      path: '/db/:db',
      component: BasicLayout,
      children: [
        { path: '', name: 'db', component: DbView },
        { path: 'table/:t', name: 'db-table', component: TableView },
        { path: 'console', name: 'db-console', component: ConsoleView },
        { path: 'export', name: 'db-export', component: ExportView },
        { path: 'import', name: 'db-import', component: ImportView },
      ],
    },
  ],
})

// Guard: without a session every navigation lands on /login; with a
// session /login bounces to /server. When the store has no session yet
// (e.g. a page reload with a live cookie) it is probed via GET
// /api/session, but only once: after a failed probe the store's
// authChecked flag makes further probes pointless (the cookie is gone or
// the server is unreachable) and they are skipped until login or logout
// resets the flag.
router.beforeEach(async (to) => {
  const store = useSessionStore()
  if (store.sessionInfo === null && !store.authChecked) {
    try {
      await store.fetchSession()
    } catch {
      // No or expired session cookie, or the server is unreachable.
      store.authChecked = true
    }
  }
  if (store.sessionInfo === null) {
    return to.path === '/login' ? true : '/login'
  }
  const catalog = useCatalogStore()
  catalog.schemaAware = store.sessionInfo.caps.schemas
  catalog.schema = catalog.schemaAware ? (typeof to.query.schema === 'string' ? to.query.schema : 'public') : null
  return to.path === '/login' ? '/server' : true
})

export default router

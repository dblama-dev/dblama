import { createRouter, createWebHistory } from 'vue-router'

import BasicLayout from '../layouts/BasicLayout.vue'
import { useSessionStore } from '../stores/session'
import LoginView from '../views/LoginView.vue'
import ServerView from '../views/ServerView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/server' },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/server',
      component: BasicLayout,
      children: [{ path: '', name: 'server', component: ServerView }],
    },
  ],
})

// Guard: without a session every navigation lands on /login; with a
// session /login bounces to /server. When the store has no session yet
// (e.g. a page reload with a live cookie) it is probed once per
// navigation via GET /api/session.
router.beforeEach(async (to) => {
  const store = useSessionStore()
  if (store.sessionInfo === null) {
    try {
      await store.fetchSession()
    } catch {
      // No or expired session cookie, or the server is unreachable.
    }
  }
  if (store.sessionInfo === null) {
    return to.path === '/login' ? true : '/login'
  }
  return to.path === '/login' ? '/server' : true
})

export default router

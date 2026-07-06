import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './api'

// Lazy-loaded routes: each view is a separate chunk, so the initial bundle stays
// small and a route's code only loads when visited.
const routes = [
  { path: '/login', name: 'login', component: () => import('./views/Login.vue') },
  { path: '/', name: 'shows', component: () => import('./views/Shows.vue') },
  { path: '/shows/:id', name: 'show', component: () => import('./views/ShowDetail.vue') },
  { path: '/episodes/:id', name: 'episode', component: () => import('./views/Episode.vue') },
  { path: '/search', name: 'search', component: () => import('./views/Search.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Every route except /login requires a token.
router.beforeEach((to) => {
  if (to.name !== 'login' && !getToken()) {
    return { name: 'login' }
  }
})

export default router

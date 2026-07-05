import { createRouter, createWebHistory } from 'vue-router'

// Lazy-loaded routes: each view is a separate chunk, so the initial bundle stays
// small and a route's code only loads when visited.
const routes = [
  { path: '/', name: 'search', component: () => import('./views/Search.vue') },
  { path: '/episodes/:id', name: 'episode', component: () => import('./views/Episode.vue') },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})

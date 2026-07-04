import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    // Домашний экран — «Исследования» (список прогонов), приземление после входа.
    {
      path: '/',
      name: 'research',
      component: () => import('@/views/ResearchView.vue'),
      meta: { crumb: 'Исследования' },
    },
    {
      path: '/generator/:runId?',
      name: 'generator',
      component: () => import('@/views/GeneratorView.vue'),
      props: true,
      meta: { crumb: 'Генератор' },
    },
    {
      path: '/knowledge',
      name: 'knowledge',
      component: () => import('@/views/KnowledgeView.vue'),
      meta: { crumb: 'База знаний' },
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

// Гард: всё, кроме входа, требует локальной авторизации.
router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'research' }
  }
})

export default router

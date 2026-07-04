import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', redirect: '/runs' },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/generator',
      name: 'generator',
      component: () => import('@/views/GeneratorView.vue'),
      meta: { public: true },
    },
    {
      path: '/runs',
      name: 'runs',
      component: () => import('@/views/RunsListView.vue'),
    },
    {
      path: '/runs/new',
      name: 'run-new',
      component: () => import('@/views/RunCreateView.vue'),
    },
    {
      path: '/runs/:runId',
      name: 'run-detail',
      component: () => import('@/views/RunDetailView.vue'),
      props: true,
    },
    {
      path: '/documents',
      name: 'documents',
      component: () => import('@/views/DocumentsView.vue'),
    },
  ],
})

// Гард: закрытые маршруты требуют авторизации; на /login не пускаем
// уже вошедших. Стор доступен здесь, т.к. Pinia установлена до router (main.ts).
router.beforeEach((to) => {
  const auth = useAuthStore()

  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'runs' }
  }
})

export default router

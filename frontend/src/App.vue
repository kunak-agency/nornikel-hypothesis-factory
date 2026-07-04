<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { Toaster } from 'vue-sonner'

import { useTheme } from '@/composables/useTheme'
import { useAuthStore } from '@/stores/auth'

const { isDark, toggle } = useTheme()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

// На публичных страницах (логин) прячем оболочку — вьюха рисует себя целиком.
const showChrome = computed(() => !route.meta.public)

function onLogout() {
  auth.logout()
  router.push({ name: 'login' })
}

const links = [
  { to: '/runs', label: 'Прогоны', icon: 'lucide:flask-conical' },
  { to: '/runs/new', label: 'Новый прогон', icon: 'lucide:plus' },
  { to: '/documents', label: 'База знаний', icon: 'lucide:book-open' },
]
</script>

<template>
  <div class="min-h-screen">
    <header
      v-if="showChrome"
      class="flex items-center justify-between gap-6 border-b border-border px-6 py-4"
    >
      <RouterLink to="/" class="flex items-center gap-2 text-lg font-bold text-text">
        <Icon icon="lucide:atom" class="size-5 text-brand" />
        Фабрика гипотез
      </RouterLink>

      <nav class="flex items-center gap-1">
        <RouterLink
          v-for="link in links"
          :key="link.to"
          :to="link.to"
          class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm text-muted transition-colors hover:bg-surface hover:text-text"
          active-class="!text-brand font-semibold"
        >
          <Icon :icon="link.icon" class="size-4" />
          {{ link.label }}
        </RouterLink>

        <button
          type="button"
          class="ml-2 flex size-9 items-center justify-center rounded-md border border-border bg-surface text-muted transition-colors hover:text-text"
          :aria-label="isDark ? 'Светлая тема' : 'Тёмная тема'"
          @click="toggle"
        >
          <Icon :icon="isDark ? 'lucide:sun' : 'lucide:moon'" class="size-4" />
        </button>

        <div v-if="auth.user" class="ml-2 flex items-center gap-2 border-l border-border pl-3">
          <span
            class="flex size-8 items-center justify-center rounded-full bg-brand/15 text-xs font-semibold text-brand"
            :title="auth.user.email"
          >
            {{ auth.user.name.charAt(0).toUpperCase() }}
          </span>
          <span class="hidden text-sm text-muted sm:inline">{{ auth.user.name }}</span>
          <button
            type="button"
            class="flex size-9 items-center justify-center rounded-md border border-border bg-surface text-muted transition-colors hover:text-red-500"
            aria-label="Выйти"
            title="Выйти"
            @click="onLogout"
          >
            <Icon icon="lucide:log-out" class="size-4" />
          </button>
        </div>
      </nav>
    </header>

    <main v-if="showChrome" class="mx-auto max-w-5xl px-6 py-6">
      <RouterView />
    </main>
    <RouterView v-else />

    <Toaster position="top-right" rich-colors close-button />
  </div>
</template>

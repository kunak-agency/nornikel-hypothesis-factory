<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { Toaster } from 'vue-sonner'

import { useAuthStore } from '@/stores/auth'
import ModelSwitcher from '@/components/ui/ModelSwitcher.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

// На публичных страницах (вход) оболочку не рисуем — вьюха рисует себя целиком.
const showShell = computed(() => !route.meta.public)

// Пункты бокового меню «Платформа» (спец. 01): Исследования → Генератор → База знаний.
const nav = [
  { to: '/', label: 'Исследования', icon: 'lucide:flask-conical' },
  { to: '/generator', label: 'Генератор', icon: 'lucide:lightbulb' },
  { to: '/knowledge', label: 'База знаний', icon: 'lucide:database' },
]

// Хлебная крошка контекста для верхней панели.
const crumb = computed(() => (route.meta.crumb as string) || 'Исследования')

function onLogout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <template v-if="showShell">
    <div class="flex h-screen">
      <!-- SIDEBAR -->
      <aside class="flex w-[248px] shrink-0 flex-col border-r border-bd bg-white">
        <RouterLink to="/" class="flex h-14 items-center border-b border-bd px-4">
          <img src="/nornickel.png" alt="НОРНИКЕЛЬ" class="h-[26px]" />
        </RouterLink>

        <nav class="flex flex-col gap-0.5 px-3 pt-4">
          <div class="lbl px-2.5 pb-1.5">Платформа</div>
          <RouterLink
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            class="nav-i"
            active-class="on"
            :aria-current="route.path === item.to ? 'page' : undefined"
          >
            <Icon :icon="item.icon" class="size-[17px]" />
            {{ item.label }}
          </RouterLink>
        </nav>

        <div class="mt-auto p-3">
          <!-- Защищённый контур — постоянное напоминание о закрытом периметре. -->
          <div class="rounded-xl border border-bd bg-white p-3">
            <div class="mb-1.5 flex items-center gap-1.5">
              <span class="size-1.5 rounded-full bg-ok"></span>
              <span class="text-[12px] font-semibold text-body">Защищённый контур</span>
            </div>
            <div class="text-[11px] leading-relaxed text-faint">Данные не покидают периметр.</div>
          </div>

          <!-- Блок пользователя (статичный: ролей и авторизации на бэке нет). -->
          <button
            type="button"
            class="mt-2 flex w-full items-center gap-2.5 rounded-[10px] px-2 py-2 transition hover:bg-muted"
            @click="onLogout"
            title="Выйти"
          >
            <span
              class="mono grid size-8 place-items-center rounded-full bg-ink text-[11px] font-semibold text-white"
            >
              {{ auth.user.initials }}
            </span>
            <span class="flex-1 text-left leading-tight">
              <span class="block text-[13px] font-semibold text-ink">{{ auth.user.name }}</span>
              <span class="block text-[11px] text-faint">{{ auth.user.role }}</span>
            </span>
            <Icon icon="lucide:log-out" class="size-[15px] text-faint" />
          </button>
        </div>
      </aside>

      <!-- MAIN -->
      <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header class="flex h-14 shrink-0 items-center gap-4 border-b border-bd bg-bg px-7">
          <div class="flex items-center gap-2 text-[13px]">
            <span class="text-faint">Проект</span>
            <span class="text-faint">/</span>
            <span class="font-medium text-body">{{ crumb }}</span>
          </div>
          <div class="flex-1"></div>
          <ModelSwitcher />
        </header>

        <main class="flex-1 overflow-y-auto px-7 py-7">
          <RouterView />
        </main>
      </div>
    </div>
  </template>

  <RouterView v-else />

  <Toaster position="top-right" rich-colors close-button />
</template>

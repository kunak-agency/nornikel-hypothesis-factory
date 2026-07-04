<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/composables/useTheme'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { isDark, toggle } = useTheme()

const showPassword = ref(false)

const schema = toTypedSchema(
  z.object({
    email: z.string().min(1, 'Укажите email').email('Некорректный email'),
    password: z.string().min(6, 'Минимум 6 символов'),
  }),
)

const { handleSubmit, errors, defineField, isSubmitting } = useForm({
  validationSchema: schema,
  initialValues: { email: '', password: '' },
})

const [email, emailAttrs] = defineField('email')
const [password, passwordAttrs] = defineField('password')

const onSubmit = handleSubmit(async (values) => {
  try {
    await auth.login(values.email, values.password)
    toast.success('С возвращением!')
    const redirect = (route.query.redirect as string) || '/runs'
    router.push(redirect)
  } catch (e) {
    toast.error('Не удалось войти', { description: (e as Error).message })
  }
})
</script>

<template>
  <div class="grid min-h-screen lg:grid-cols-2">
    <!-- Брендовая панель (декоративная, скрыта на мобильных) -->
    <aside
      class="relative hidden overflow-hidden bg-gradient-to-br from-[oklch(0.55_0.2_262)] to-[oklch(0.42_0.19_262)] text-brand-fg lg:flex lg:flex-col lg:justify-between lg:p-12"
    >
      <!-- Декор: орбиты + узлы графа гипотез -->
      <svg
        class="pointer-events-none absolute inset-0 h-full w-full opacity-25"
        viewBox="0 0 400 400"
        fill="none"
        aria-hidden="true"
      >
        <g stroke="currentColor" stroke-width="0.75">
          <circle cx="300" cy="120" r="90" />
          <circle cx="300" cy="120" r="150" />
          <circle cx="300" cy="120" r="210" />
          <line x1="300" y1="120" x2="120" y2="300" />
          <line x1="300" y1="120" x2="360" y2="330" />
          <line x1="120" y1="300" x2="360" y2="330" />
        </g>
        <g fill="currentColor">
          <circle cx="300" cy="120" r="5" />
          <circle cx="120" cy="300" r="4" />
          <circle cx="360" cy="330" r="4" />
          <circle cx="390" cy="30" r="3" />
        </g>
      </svg>

      <div class="relative flex items-center gap-2 text-lg font-bold">
        <Icon icon="lucide:atom" class="size-6" />
        Фабрика гипотез
      </div>

      <div class="relative max-w-md">
        <h2 class="text-3xl font-bold leading-tight">
          Гипотезы для обогащения — из вашей базы знаний
        </h2>
        <p class="mt-4 text-brand-fg/80">
          Опишите производственную проблему — система подберёт релевантные
          источники и сгенерирует проверяемые гипотезы с обоснованием.
        </p>
      </div>

      <div class="relative flex items-center gap-6 text-sm text-brand-fg/70">
        <span class="flex items-center gap-1.5">
          <Icon icon="lucide:book-open" class="size-4" /> База знаний
        </span>
        <span class="flex items-center gap-1.5">
          <Icon icon="lucide:git-fork" class="size-4" /> Граф связей
        </span>
        <span class="flex items-center gap-1.5">
          <Icon icon="lucide:file-check-2" class="size-4" /> Отчёты
        </span>
      </div>
    </aside>

    <!-- Панель формы -->
    <main class="relative flex items-center justify-center px-6 py-12">
      <button
        type="button"
        class="absolute right-6 top-6 flex size-9 items-center justify-center rounded-md border border-border bg-surface text-muted transition-colors hover:text-text"
        :aria-label="isDark ? 'Светлая тема' : 'Тёмная тема'"
        @click="toggle"
      >
        <Icon :icon="isDark ? 'lucide:sun' : 'lucide:moon'" class="size-4" />
      </button>

      <div class="w-full max-w-sm">
        <!-- Лого для мобильных, где брендовая панель скрыта -->
        <div class="mb-8 flex items-center gap-2 text-lg font-bold lg:hidden">
          <Icon icon="lucide:atom" class="size-6 text-brand" />
          Фабрика гипотез
        </div>

        <h1 class="text-2xl font-bold">Вход в систему</h1>
        <p class="mt-1.5 text-sm text-muted">
          Введите свои данные, чтобы продолжить работу
        </p>

        <form class="mt-8 space-y-5" novalidate @submit="onSubmit">
          <div class="space-y-1.5">
            <label for="email" class="text-sm font-medium">Email</label>
            <div class="relative">
              <Icon
                icon="lucide:mail"
                class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted"
              />
              <input
                id="email"
                v-model="email"
                v-bind="emailAttrs"
                type="email"
                autocomplete="email"
                placeholder="you@nornickel.ru"
                class="w-full rounded-md border border-border bg-bg py-2 pl-9 pr-3 text-sm outline-none focus:ring-2 focus:ring-brand/40"
                :class="{ 'border-red-500 focus:ring-red-500/30': errors.email }"
              />
            </div>
            <p v-if="errors.email" class="flex items-center gap-1 text-sm text-red-500">
              <Icon icon="lucide:circle-alert" class="size-4" />
              {{ errors.email }}
            </p>
          </div>

          <div class="space-y-1.5">
            <div class="flex items-center justify-between">
              <label for="password" class="text-sm font-medium">Пароль</label>
              <a
                href="#"
                class="text-sm text-brand hover:underline"
                @click.prevent="toast.info('Обратитесь к администратору системы')"
              >
                Забыли пароль?
              </a>
            </div>
            <div class="relative">
              <Icon
                icon="lucide:lock"
                class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted"
              />
              <input
                id="password"
                v-model="password"
                v-bind="passwordAttrs"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="current-password"
                placeholder="••••••••"
                class="w-full rounded-md border border-border bg-bg py-2 pl-9 pr-10 text-sm outline-none focus:ring-2 focus:ring-brand/40"
                :class="{ 'border-red-500 focus:ring-red-500/30': errors.password }"
              />
              <button
                type="button"
                class="absolute right-2 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded text-muted transition-colors hover:text-text"
                :aria-label="showPassword ? 'Скрыть пароль' : 'Показать пароль'"
                @click="showPassword = !showPassword"
              >
                <Icon :icon="showPassword ? 'lucide:eye-off' : 'lucide:eye'" class="size-4" />
              </button>
            </div>
            <p v-if="errors.password" class="flex items-center gap-1 text-sm text-red-500">
              <Icon icon="lucide:circle-alert" class="size-4" />
              {{ errors.password }}
            </p>
          </div>

          <button
            type="submit"
            :disabled="isSubmitting"
            class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand px-4 py-2.5 text-sm font-medium text-brand-fg transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            <Icon
              :icon="isSubmitting ? 'lucide:loader-circle' : 'lucide:log-in'"
              class="size-4"
              :class="{ 'animate-spin': isSubmitting }"
            />
            {{ isSubmitting ? 'Вход…' : 'Войти' }}
          </button>
        </form>

        <p class="mt-8 text-center text-xs text-muted">
          Доступ предоставляется администратором. По вопросам входа обратитесь в
          поддержку.
        </p>
      </div>
    </main>
  </div>
</template>

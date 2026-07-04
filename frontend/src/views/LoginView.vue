<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import { Icon } from '@iconify/vue'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const badCreds = ref(false)

const schema = toTypedSchema(
  z.object({
    login: z.string().min(1, 'Укажите логин'),
    password: z.string().min(1, 'Укажите пароль'),
  }),
)

const { handleSubmit, errors, defineField } = useForm({
  validationSchema: schema,
  initialValues: { login: '', password: '' },
})

const [login, loginAttrs] = defineField('login')
const [password, passwordAttrs] = defineField('password')

const onSubmit = handleSubmit((values) => {
  badCreds.value = false
  if (auth.login(values.login, values.password)) {
    router.push((route.query.redirect as string) || '/')
  } else {
    badCreds.value = true
  }
})
</script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center gap-8 bg-bg px-4">
    <img src="/nornickel.png" alt="НОРНИКЕЛЬ" class="h-8" />

    <form class="card w-full max-w-[380px] p-7" novalidate @submit="onSubmit">
      <h1 class="h2 mb-6 text-[19px] font-bold text-ink">Вход в платформу</h1>

      <div class="mb-4 space-y-1.5">
        <label for="login" class="lbl">Логин</label>
        <input
          id="login"
          v-model="login"
          v-bind="loginAttrs"
          type="text"
          autocomplete="username"
          class="inp"
          :class="{ '!border-danger': errors.login || badCreds }"
          placeholder="Логин"
          @input="badCreds = false"
        />
        <p v-if="errors.login" class="text-[12px] text-danger">{{ errors.login }}</p>
      </div>

      <div class="mb-4 space-y-1.5">
        <label for="password" class="lbl">Пароль</label>
        <input
          id="password"
          v-model="password"
          v-bind="passwordAttrs"
          type="password"
          autocomplete="current-password"
          class="inp"
          :class="{ '!border-danger': errors.password || badCreds }"
          placeholder="Пароль"
          @input="badCreds = false"
        />
        <p v-if="errors.password" class="text-[12px] text-danger">{{ errors.password }}</p>
      </div>

      <p
        v-if="badCreds"
        class="mb-4 flex items-center gap-1.5 text-[13px] text-danger"
      >
        <Icon icon="lucide:circle-alert" class="size-4" />
        Неверный логин или пароль
      </p>

      <button type="submit" class="btn btn-primary w-full py-2.5 text-[14px]">Войти</button>
    </form>

    <div class="flex items-center gap-1.5 text-[12.5px] text-faint">
      <span class="size-1.5 rounded-full bg-ok"></span>
      Защищённый контур — данные не покидают периметр
    </div>
  </div>
</template>

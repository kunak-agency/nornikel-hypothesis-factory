<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import AppSelect from '@/components/ui/AppSelect.vue'
import { useRunsStore } from '@/stores/runs'

const store = useRunsStore()
const router = useRouter()

const LANGUAGES = [
  { value: 'ru', label: 'Русский' },
  { value: 'en', label: 'English' },
  { value: 'zh', label: '中文' },
]

// Схема валидации формы (zod → typed schema для vee-validate).
const schema = toTypedSchema(
  z.object({
    rawText: z
      .string()
      .min(20, 'Опишите проблему подробнее (минимум 20 символов)'),
    language: z.enum(['ru', 'en', 'zh']),
  }),
)

const { handleSubmit, errors, defineField, isSubmitting } = useForm({
  validationSchema: schema,
  initialValues: { rawText: '', language: 'ru' },
})

const [rawText, rawTextAttrs] = defineField('rawText')
const [language] = defineField('language')

const onSubmit = handleSubmit(async (values) => {
  try {
    const run = await store.createRun({
      rawText: values.rawText,
      language: values.language,
    })
    toast.success('Прогон запущен')
    router.push(`/runs/${run.id}`)
  } catch (e) {
    toast.error('Не удалось запустить прогон', { description: (e as Error).message })
  }
})
</script>

<template>
  <section class="space-y-6">
    <h1 class="text-2xl font-bold">Новый прогон</h1>

    <form class="space-y-5" @submit="onSubmit">
      <div class="space-y-1.5">
        <label for="rawText" class="text-sm font-medium">
          Описание проблемы (ProblemSpec, свободный текст)
        </label>
        <textarea
          id="rawText"
          v-model="rawText"
          v-bind="rawTextAttrs"
          rows="10"
          class="w-full rounded-md border border-border bg-bg px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-brand/40"
          placeholder="Фабрика: КГМК. Породные хвосты… Потери Ni/Cu по крупности…"
        />
        <p v-if="errors.rawText" class="flex items-center gap-1 text-sm text-red-500">
          <Icon icon="lucide:circle-alert" class="size-4" />
          {{ errors.rawText }}
        </p>
      </div>

      <div class="max-w-56 space-y-1.5">
        <label class="text-sm font-medium">Язык отчёта</label>
        <AppSelect v-model="language" :options="LANGUAGES" />
      </div>

      <button
        type="submit"
        :disabled="isSubmitting"
        class="inline-flex items-center gap-2 rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-fg transition-opacity hover:opacity-90 disabled:opacity-50"
      >
        <Icon
          :icon="isSubmitting ? 'lucide:loader-circle' : 'lucide:play'"
          class="size-4"
          :class="{ 'animate-spin': isSubmitting }"
        />
        {{ isSubmitting ? 'Запуск…' : 'Запустить' }}
      </button>
    </form>
  </section>
</template>

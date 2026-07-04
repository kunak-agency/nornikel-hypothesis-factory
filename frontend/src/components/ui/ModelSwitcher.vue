<script setup lang="ts">
import {
  SelectContent,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectViewport,
} from 'reka-ui'
import { ref, computed } from 'vue'
import { Icon } from '@iconify/vue'

// ⚠️ Мок для демо: реальный бэкенд ходит только в YandexGPT (yandexgpt/latest).
// Переключатель НЕ меняет модель на сервере — это витрина «мультимодельности»
// для показа. Выбор запоминается локально, чтобы демо выглядело живым.
interface Model {
  value: string
  label: string
  provider: string
  slug: string
  real?: boolean
}

const MODELS: Model[] = [
  { value: 'yandexgpt/latest', label: 'YandexGPT', provider: 'Yandex', slug: 'yandexgpt/latest', real: true },
  { value: 'openai/gpt-5.5', label: 'GPT-5.5', provider: 'OpenAI', slug: 'openai/gpt-5.5' },
  { value: 'openai/gpt-chat-latest-20260505', label: 'GPT Chat', provider: 'OpenAI', slug: 'openai/gpt-chat-latest' },
  { value: 'anthropic/claude-opus-4.8', label: 'Claude Opus 4.8', provider: 'Anthropic', slug: 'anthropic/claude-opus-4.8' },
  { value: 'z-ai/glm-5.2', label: 'GLM-5.2', provider: 'Z-AI', slug: 'z-ai/glm-5.2' },
  { value: 'deepseek/deepseek-v4-pro', label: 'DeepSeek V4 Pro', provider: 'DeepSeek', slug: 'deepseek/deepseek-v4-pro' },
  { value: 'perplexity/sonar-pro', label: 'Sonar Pro', provider: 'Perplexity', slug: 'perplexity/sonar-pro' },
  { value: 'google/gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro', provider: 'Google', slug: 'google/gemini-3.1-pro-preview' },
]

const STORAGE_KEY = 'hf-model'
const model = ref<string>(localStorage.getItem(STORAGE_KEY) || MODELS[0].value)
const current = computed(() => MODELS.find((m) => m.value === model.value) ?? MODELS[0])

function onChange(v: string) {
  model.value = v
  localStorage.setItem(STORAGE_KEY, v)
}
</script>

<template>
  <SelectRoot :model-value="model" @update:model-value="onChange">
    <SelectTrigger
      class="btn btn-secondary px-3 py-1.5 text-[12.5px]"
      title="Активная модель (демо-переключатель — генерация всегда идёт через YandexGPT)"
    >
      <span class="size-1.5 rounded-full" :class="current.real ? 'bg-ok' : 'bg-warn'"></span>
      <span class="mono">{{ current.label }}</span>
      <Icon icon="lucide:chevron-down" class="size-3.5 text-faint" />
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        position="popper"
        align="end"
        :side-offset="6"
        class="z-50 w-64 overflow-hidden rounded-md border border-bds bg-white shadow-lg"
      >
        <SelectViewport class="p-1">
          <div class="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-faint">
            Модель генерации
          </div>
          <SelectItem
            v-for="m in MODELS"
            :key="m.value"
            :value="m.value"
            class="flex cursor-pointer items-center justify-between gap-2 rounded px-2 py-1.5 text-sm text-body outline-none data-[highlighted]:bg-accentbg data-[highlighted]:text-accent"
          >
            <span class="flex min-w-0 flex-col">
              <SelectItemText class="truncate font-medium">{{ m.label }}</SelectItemText>
              <span class="truncate font-mono text-[11px] text-faint">{{ m.slug }}</span>
            </span>
            <SelectItemIndicator>
              <Icon icon="lucide:check" class="size-4 shrink-0 text-accent" />
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

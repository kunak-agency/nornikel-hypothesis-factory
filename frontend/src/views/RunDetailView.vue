<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Icon } from '@iconify/vue'

import { runsApi } from '@/api/runs'
import { useRunsStore } from '@/stores/runs'

const props = defineProps<{ runId: string }>()

const store = useRunsStore()
const { current, error } = storeToRefs(store)

let cancelled = false

const isTerminal = computed(() => ['done', 'failed'].includes(current.value?.status ?? ''))
const reportUrl = ref('')

onMounted(async () => {
  reportUrl.value = runsApi.reportMarkdownUrl(props.runId)
  await store.fetchRun(props.runId)
  if (!cancelled && !isTerminal.value) {
    store.pollRun(props.runId)
  }
})

onUnmounted(() => {
  cancelled = true
})
</script>

<template>
  <section v-if="current" class="space-y-5">
    <div class="flex items-center gap-3">
      <h1 class="text-2xl font-bold">{{ current.problemSpec.plant || 'Прогон' }}</h1>
      <span
        class="rounded-full px-2 py-0.5 text-xs font-medium"
        :class="
          isTerminal
            ? current.status === 'done'
              ? 'bg-green-500/15 text-green-600 dark:text-green-400'
              : 'bg-red-500/15 text-red-600 dark:text-red-400'
            : 'bg-brand/15 text-brand'
        "
      >
        <Icon
          v-if="!isTerminal"
          icon="lucide:loader-circle"
          class="mr-1 inline size-3 animate-spin"
        />
        {{ current.status }}
      </span>
    </div>

    <p class="text-muted">{{ current.problemSpec.targetKpi }}</p>

    <p v-if="current.error" class="text-sm text-red-500">Ошибка прогона: {{ current.error }}</p>

    <a
      v-if="isTerminal && current.status === 'done'"
      :href="reportUrl"
      target="_blank"
      class="inline-flex items-center gap-2 rounded-md border border-border bg-surface px-4 py-2 text-sm hover:border-brand/50"
    >
      <Icon icon="lucide:file-text" class="size-4" />
      Отчёт (Markdown)
    </a>

    <div
      v-if="current.knowledgeGaps?.length"
      class="rounded-card border border-amber-500/30 bg-amber-500/10 p-3 text-sm"
    >
      <strong class="font-semibold">Пробелы в знаниях:</strong>
      {{ current.knowledgeGaps.join(', ') }}
    </div>

    <template v-if="current.hypotheses?.length">
      <h2 class="text-lg font-semibold">Гипотезы</h2>
      <ol class="space-y-3">
        <li
          v-for="h in current.hypotheses"
          :key="h.id"
          class="space-y-2 rounded-card border border-border bg-surface p-4"
        >
          <div class="flex items-start justify-between gap-3">
            <strong class="font-semibold">#{{ h.rank }} — {{ h.statement }}</strong>
            <span class="shrink-0 rounded-full bg-brand/15 px-2 py-0.5 text-xs font-medium text-brand">
              total {{ h.scores.total.toFixed(2) }}
            </span>
          </div>
          <p class="text-sm text-muted">{{ h.mechanism }}</p>
          <div class="flex flex-wrap gap-1.5 text-xs">
            <span class="rounded-full bg-border px-2 py-0.5 text-muted">
              novelty {{ h.scores.novelty.toFixed(2) }}
            </span>
            <span class="rounded-full bg-border px-2 py-0.5 text-muted">
              impact {{ h.scores.impact.toFixed(2) }}
            </span>
            <span class="rounded-full bg-border px-2 py-0.5 text-muted">
              feasibility {{ h.scores.feasibility.toFixed(2) }}
            </span>
          </div>
        </li>
      </ol>
    </template>
  </section>

  <p v-else-if="error" class="text-muted">Ошибка: {{ error }}</p>
  <p v-else class="text-muted">Загрузка…</p>
</template>

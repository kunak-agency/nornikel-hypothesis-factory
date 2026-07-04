<script setup lang="ts">
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { Icon } from '@iconify/vue'

import { useRunsStore } from '@/stores/runs'

const store = useRunsStore()
const { runs, loading, error } = storeToRefs(store)

// Цвет бейджа статуса по состоянию прогона.
function statusClass(status: string): string {
  if (status === 'done') return 'bg-green-500/15 text-green-600 dark:text-green-400'
  if (status === 'failed') return 'bg-red-500/15 text-red-600 dark:text-red-400'
  return 'bg-brand/15 text-brand'
}

onMounted(() => store.fetchRuns())
</script>

<template>
  <section class="space-y-5">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold">Прогоны</h1>
      <RouterLink
        to="/runs/new"
        class="inline-flex items-center gap-2 rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-fg hover:opacity-90"
      >
        <Icon icon="lucide:plus" class="size-4" />
        Новый прогон
      </RouterLink>
    </div>

    <p v-if="loading" class="text-muted">Загрузка…</p>
    <p v-else-if="error" class="text-muted">Ошибка: {{ error }}</p>
    <p v-else-if="!runs.length" class="text-muted">Пока нет прогонов.</p>

    <ul v-else class="space-y-3">
      <li
        v-for="run in runs"
        :key="run.id"
        class="rounded-card border border-border bg-surface p-4 transition-colors hover:border-brand/50"
      >
        <RouterLink :to="`/runs/${run.id}`" class="flex items-center gap-3">
          <span class="font-semibold text-text">
            {{ run.problemSpec.plant || 'Без названия' }}
          </span>
          <span
            class="rounded-full px-2 py-0.5 text-xs font-medium"
            :class="statusClass(run.status)"
          >
            {{ run.status }}
          </span>
        </RouterLink>
        <p class="mt-1 text-sm text-muted">{{ run.problemSpec.targetKpi }}</p>
      </li>
    </ul>
  </section>
</template>

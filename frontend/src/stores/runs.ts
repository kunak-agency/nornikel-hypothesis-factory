import { defineStore } from 'pinia'
import { ref } from 'vue'

import { runsApi, type ExcelRunOptions } from '@/api/runs'
import type { CreateRunRequest, Run } from '@/api/types'

// Терминальные статусы прогона — на них поллинг останавливается.
const TERMINAL_STATUSES = new Set(['done', 'failed'])
export const isTerminal = (s: string) => TERMINAL_STATUSES.has(s)

export const useRunsStore = defineStore('runs', () => {
  const runs = ref<Run[]>([])
  const total = ref(0)
  const current = ref<Run | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchRuns(page = 1, perPage = 30) {
    loading.value = true
    error.value = null
    try {
      const resp = await runsApi.list(page, perPage)
      runs.value = resp.items ?? []
      total.value = resp.total ?? runs.value.length
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function fetchRun(runId: string) {
    loading.value = true
    error.value = null
    try {
      current.value = await runsApi.get(runId)
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function createRun(body: CreateRunRequest): Promise<Run> {
    const run = await runsApi.create(body)
    current.value = run
    return run
  }

  async function createRunFromExcel(
    file: File,
    rawText: string,
    opts?: ExcelRunOptions,
  ): Promise<Run> {
    const run = await runsApi.createFromExcel(file, rawText, opts)
    current.value = run
    return run
  }

  async function removeRun(runId: string): Promise<void> {
    await runsApi.remove(runId)
    runs.value = runs.value.filter((r) => r.id !== runId)
    total.value = Math.max(0, total.value - 1)
  }

  // Поллинг GET /v1/runs/{id} пока статус не станет терминальным.
  async function pollRun(
    runId: string,
    onTick?: (run: Run) => void,
    intervalMs = 2500,
  ): Promise<Run> {
     
    while (true) {
      const run = await runsApi.get(runId)
      current.value = run
      onTick?.(run)
      if (TERMINAL_STATUSES.has(run.status)) return run
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }
  }

  // Обновить незавершённые строки списка (живые бейджи/стептеры) одним проходом.
  // Возвращает true, если остались незавершённые прогоны (нужен следующий тик).
  async function refreshInProgress(): Promise<boolean> {
    const pending = runs.value.filter((r) => !TERMINAL_STATUSES.has(r.status))
    if (!pending.length) return false
    await Promise.all(
      pending.map(async (r) => {
        try {
          const fresh = await runsApi.get(r.id)
          const idx = runs.value.findIndex((x) => x.id === r.id)
          if (idx !== -1) runs.value[idx] = { ...runs.value[idx], ...fresh }
        } catch {
          /* игнорируем единичные сбои — повтор на следующем тике */
        }
      }),
    )
    return runs.value.some((r) => !TERMINAL_STATUSES.has(r.status))
  }

  return {
    runs,
    total,
    current,
    loading,
    error,
    fetchRuns,
    fetchRun,
    createRun,
    createRunFromExcel,
    removeRun,
    pollRun,
    refreshInProgress,
  }
})

import { defineStore } from 'pinia'
import { ref } from 'vue'

import { runsApi } from '@/api/runs'
import type { CreateRunRequest, Run } from '@/api/types'

// Терминальные статусы прогона — на них поллинг останавливается.
const TERMINAL_STATUSES = new Set(['done', 'failed'])

export const useRunsStore = defineStore('runs', () => {
  const runs = ref<Run[]>([])
  const current = ref<Run | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchRuns(page = 1, perPage = 20) {
    loading.value = true
    error.value = null
    try {
      const resp = await runsApi.list(page, perPage)
      runs.value = resp.items
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

  // Поллинг GET /v1/runs/{id} пока статус не станет терминальным.
  async function pollRun(runId: string, intervalMs = 2500): Promise<Run> {
     
    while (true) {
      const run = await runsApi.get(runId)
      current.value = run
      if (TERMINAL_STATUSES.has(run.status)) return run
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }
  }

  return { runs, current, loading, error, fetchRuns, fetchRun, createRun, pollRun }
})

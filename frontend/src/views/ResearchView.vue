<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { useRunsStore, isTerminal } from '@/stores/runs'
import { entitiesApi } from '@/api/entities'
import type { Run, EntityReputation } from '@/api/types'
import StatusBadge from '@/components/StatusBadge.vue'

const store = useRunsStore()
const router = useRouter()

type Filter = 'all' | 'in_progress' | 'done'
const filter = ref<Filter>('all')
const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'Все' },
  { key: 'in_progress', label: 'В работе' },
  { key: 'done', label: 'Готово' },
]

let timer: ReturnType<typeof setTimeout> | null = null

// Фильтр — клиентский: GET /v1/runs не принимает status.
const filteredRuns = computed(() => {
  if (filter.value === 'all') return store.runs
  if (filter.value === 'done') return store.runs.filter((r) => r.status === 'done')
  return store.runs.filter((r) => !isTerminal(r.status)) // «В работе» = кроме done/failed
})

async function load() {
  await store.fetchRuns(1, 30)
  scheduleRefresh()
}

// Живое автообновление незавершённых строк (лёгкий поллинг GET /v1/runs/:id).
function scheduleRefresh() {
  if (timer) clearTimeout(timer)
  const hasPending = store.runs.some((r) => !isTerminal(r.status))
  if (!hasPending) return
  timer = setTimeout(async () => {
    const stillPending = await store.refreshInProgress()
    if (stillPending) scheduleRefresh()
  }, 3000)
}

function setFilter(f: Filter) {
  filter.value = f
}

function openRun(run: Run) {
  router.push(`/generator/${run.id}`)
}

// --- Удаление прогона (каскадное, DELETE /v1/runs/:id) ---
const pendingDelete = ref<Run | null>(null)
const deleting = ref(false)

function askDelete(run: Run, e: Event) {
  e.stopPropagation()
  pendingDelete.value = run
}
async function confirmDelete() {
  if (!pendingDelete.value) return
  deleting.value = true
  try {
    await store.removeRun(pendingDelete.value.id)
    toast.success('Прогон удалён')
    pendingDelete.value = null
  } catch (e) {
    toast.error('Не удалось удалить', { description: (e as Error).message })
  } finally {
    deleting.value = false
  }
}

const isEmpty = computed(() => !store.loading && !store.error && store.runs.length === 0)
const noneInFilter = computed(
  () => !store.loading && !store.error && store.runs.length > 0 && filteredRuns.value.length === 0,
)

function fmtDate(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' })
}

// --- Репутация сущностей (GET /v1/entities/reputation) ---
const reputation = ref<EntityReputation[]>([])
const repLoading = ref(false)
const repError = ref<string | null>(null)

async function loadReputation() {
  repLoading.value = true
  repError.value = null
  try {
    const resp = await entitiesApi.reputation()
    // Наиболее «активные» сущности сверху.
    reputation.value = (resp.items ?? []).sort(
      (a, b) =>
        b.confirmed + b.rejected + b.needsRevision - (a.confirmed + a.rejected + a.needsRevision),
    )
  } catch (e) {
    repError.value = (e as Error).message
  } finally {
    repLoading.value = false
  }
}

onMounted(() => {
  load()
  loadReputation()
})
onBeforeUnmount(() => timer && clearTimeout(timer))
</script>

<template>
  <section>
    <div class="mb-6 flex items-end gap-4">
      <div>
        <h1 class="h1 mb-1 text-[26px] font-extrabold text-ink">Исследования</h1>
        <p class="text-[14px] text-sec">Все прогоны генерации гипотез, новые сверху</p>
      </div>
      <div class="flex-1"></div>

      <div class="flex items-center gap-0.5 rounded-[10px] border border-bd bg-white p-0.5">
        <button
          v-for="f in FILTERS"
          :key="f.key"
          type="button"
          class="rounded-lg px-3 py-1.5 text-[12.5px] font-medium transition"
          :class="filter === f.key ? 'bg-muted text-ink' : 'text-sec hover:text-ink'"
          @click="setFilter(f.key)"
        >
          {{ f.label }}
        </button>
      </div>

      <RouterLink to="/generator" class="btn btn-primary px-5 py-2.5 text-[14px]">
        <Icon icon="lucide:plus" class="size-4" />
        Новый прогон
      </RouterLink>
    </div>

    <!-- Загрузка -->
    <div v-if="store.loading && !store.runs.length" class="flex flex-col gap-2">
      <div v-for="i in 4" :key="i" class="card h-[62px] animate-pulse bg-muted/40"></div>
    </div>

    <!-- Ошибка -->
    <div v-else-if="store.error" class="card flex flex-col items-center gap-3 p-10 text-center">
      <Icon icon="lucide:cloud-alert" class="size-8 text-faint" />
      <p class="text-[14px] text-sec">Не удалось загрузить прогоны: {{ store.error }}</p>
      <button class="btn btn-secondary px-4 py-2 text-[13px]" @click="load">Повторить</button>
    </div>

    <!-- Пусто -->
    <div v-else-if="isEmpty" class="card flex flex-col items-center gap-2 p-12 text-center">
      <div class="mb-1 grid size-11 place-items-center rounded-full border border-bds">
        <Icon icon="lucide:flask-conical" class="size-[18px] text-faint" />
      </div>
      <div class="text-[15px] font-semibold text-ink">Пока нет ни одного прогона</div>
      <p class="text-[13.5px] text-sec">
        Создайте первый в
        <RouterLink to="/generator" class="text-accent hover:underline">Генераторе</RouterLink>.
      </p>
    </div>

    <div v-else-if="noneInFilter" class="card p-10 text-center text-[13.5px] text-sec">
      Нет прогонов в статусе «{{ FILTERS.find((f) => f.key === filter)?.label }}».
    </div>

    <!-- Список -->
    <div v-else class="card overflow-hidden">
      <div
        v-for="(run, i) in filteredRuns"
        :key="run.id"
        class="row flex w-full cursor-pointer items-center gap-4 px-5 py-3.5 text-left"
        :class="{ 'border-t border-bd': i }"
        @click="openRun(run)"
      >
        <StatusBadge :status="run.status" class="w-[132px] shrink-0" />

        <div class="min-w-0 flex-1">
          <div class="truncate text-[13.5px] font-medium text-ink">
            {{ run.problemSpec?.targetKpi || 'Без описания' }}
            <span v-if="run.problemSpec?.plant" class="text-faint">· {{ run.problemSpec.plant }}</span>
          </div>
          <div class="mt-0.5 flex items-center gap-2 text-[11.5px] text-faint">
            <span v-if="run.problemSpec?.targetMetals?.length" class="mono">
              {{ run.problemSpec.targetMetals.join(', ') }}
            </span>
            <span v-if="run.status === 'failed' && run.error" class="truncate text-danger">
              {{ run.error }}
            </span>
          </div>
        </div>

        <span class="mono shrink-0 text-[12px] text-faint">{{ fmtDate(run.createdAt) }}</span>
        <button
          class="grid size-8 shrink-0 place-items-center rounded-md text-faint transition hover:bg-muted hover:text-danger"
          title="Удалить прогон"
          @click="askDelete(run, $event)"
        >
          <Icon icon="lucide:trash-2" class="size-4" />
        </button>
        <Icon icon="lucide:chevron-right" class="size-4 shrink-0 text-bds" />
      </div>
    </div>

    <!-- Репутация сущностей (GET /v1/entities/reputation) -->
    <div class="mt-8">
      <div class="mb-2 flex items-center gap-2">
        <Icon icon="lucide:trending-up" class="size-[15px] text-faint" />
        <span class="text-[14px] font-semibold text-ink">Репутация сущностей</span>
        <span class="text-[12px] text-faint">видимая сторона обучения на обратной связи</span>
      </div>

      <div v-if="repLoading" class="card h-24 animate-pulse bg-muted/40"></div>
      <div v-else-if="repError" class="card p-5 text-[13px] text-sec">
        Не удалось загрузить репутацию: {{ repError }}
        <button class="ml-2 text-accent hover:underline" @click="loadReputation">Повторить</button>
      </div>
      <div
        v-else-if="!reputation.length"
        class="card flex items-center gap-3 p-5 text-[13px] text-sec"
      >
        <Icon icon="lucide:info" class="size-4 shrink-0 text-faint" />
        Репутация появится, когда вы начнёте оценивать гипотезы.
      </div>
      <div v-else class="card overflow-hidden">
        <div
          class="grid grid-cols-[1fr_110px_110px_120px] gap-4 border-b border-bd px-5 py-3 lbl"
        >
          <span>Сущность</span>
          <span class="text-right">Подтверждена</span>
          <span class="text-right">Отклонена</span>
          <span class="text-right">На доработке</span>
        </div>
        <div
          v-for="(e, i) in reputation"
          :key="e.entityId"
          class="grid grid-cols-[1fr_110px_110px_120px] items-center gap-4 px-5 py-2.5"
          :class="{ 'border-t border-bd': i }"
        >
          <span class="truncate text-[13.5px] text-ink">{{ e.canonicalName }}</span>
          <span class="mono text-right text-[13px] text-ok">{{ e.confirmed }}</span>
          <span class="mono text-right text-[13px] text-danger">{{ e.rejected }}</span>
          <span class="mono text-right text-[13px] text-warn">{{ e.needsRevision }}</span>
        </div>
      </div>
    </div>

    <!-- Подтверждение удаления (каскадное, необратимо) -->
    <Teleport to="body">
      <div v-if="pendingDelete" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/30" @click="pendingDelete = null"></div>
        <div class="card relative w-full max-w-[420px] p-6">
          <div class="mb-1 text-[16px] font-bold text-ink">Удалить прогон?</div>
          <p class="mb-5 text-[13px] leading-relaxed text-sec">
            Вместе с прогоном будут удалены его гипотезы, claim'ы и связанные данные.
            Действие необратимо.
          </p>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost px-4 py-2 text-[13px]" @click="pendingDelete = null">
              Отмена
            </button>
            <button
              class="btn btn-primary bg-danger px-4 py-2 text-[13px] hover:bg-danger/90"
              :disabled="deleting"
              @click="confirmDelete"
            >
              {{ deleting ? 'Удаление…' : 'Удалить' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { useRunsStore, isTerminal } from '@/stores/runs'
import type { Run } from '@/api/types'
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

const pendingDelete = ref<Run | null>(null)
const deleting = ref(false)

let timer: ReturnType<typeof setTimeout> | null = null

async function load() {
  const status = filter.value === 'all' ? undefined : filter.value
  await store.fetchRuns(1, 30, status)
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
  load()
}

function openRun(run: Run) {
  router.push(`/generator/${run.id}`)
}

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

function fmtDate(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' })
}

onMounted(load)
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

      <!-- Фильтр по статусу -->
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

    <!-- Загрузка (скелетоны) -->
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

    <!-- Список -->
    <div v-else class="card overflow-hidden">
      <button
        v-for="(run, i) in store.runs"
        :key="run.id"
        type="button"
        class="row flex w-full items-center gap-4 px-5 py-3.5 text-left"
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
            <span v-if="run.status === 'failed' && run.error" class="text-danger truncate">
              {{ run.error }}
            </span>
          </div>
        </div>

        <span class="mono shrink-0 text-[12px] text-faint">{{ fmtDate(run.createdAt) }}</span>

        <span
          class="grid size-8 shrink-0 place-items-center rounded-md text-faint transition hover:bg-muted hover:text-danger"
          title="Удалить прогон"
          @click="askDelete(run, $event)"
        >
          <Icon icon="lucide:trash-2" class="size-4" />
        </span>
        <Icon icon="lucide:chevron-right" class="size-4 shrink-0 text-bds" />
      </button>
    </div>

    <!-- Репутация сущностей — бэкенд-эндпоинта нет (заглушка) -->
    <div class="mt-8">
      <div class="mb-2 flex items-center gap-2">
        <Icon icon="lucide:trending-up" class="size-[15px] text-faint" />
        <span class="text-[14px] font-semibold text-ink">Репутация сущностей</span>
      </div>
      <div class="card flex items-center gap-3 p-5 text-[13px] text-sec">
        <Icon icon="lucide:info" class="size-4 shrink-0 text-faint" />
        Раздел появится, когда вы начнёте оценивать гипотезы — репутация копится из
        обратной связи на карточках. (Эндпоинт агрегирования пока недоступен.)
      </div>
    </div>

    <!-- Подтверждение удаления (каскадное, необратимо) -->
    <Teleport to="body">
      <div
        v-if="pendingDelete"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
      >
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

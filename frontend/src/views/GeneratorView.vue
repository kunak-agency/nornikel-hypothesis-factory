<script setup lang="ts">
import { ref, reactive, computed, onBeforeUnmount, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { useRunsStore } from '@/stores/runs'
import type { Run, CreateRunRequest } from '@/api/types'
import StageStepper from '@/components/StageStepper.vue'
import RunReport from '@/components/RunReport.vue'

const props = defineProps<{ runId?: string }>()
const store = useRunsStore()

type Phase = 'empty' | 'progress' | 'report' | 'error'
const phase = ref<Phase>('empty')
const activeRun = ref<Run | null>(null)
const busy = ref(false)
let cancelled = false

// --- Форма ---
const rawText = ref('')
const excelFile = ref<File | null>(null)
const advOpen = ref(false)
const excludedTopics = ref<string[]>([])
const excludeInput = ref('')

// Ключи соответствуют domain.RankingWeights в Swagger (evidence/risk).
const weights = reactive({
  evidence: 60,
  feasibility: 60,
  impact: 80,
  novelty: 70,
  risk: 50,
})
const WEIGHT_LABELS: Record<keyof typeof weights, string> = {
  evidence: 'Доказательность',
  feasibility: 'Реализуемость',
  impact: 'Эффект',
  novelty: 'Новизна',
  risk: 'Штраф за риск',
}

const canSubmit = computed(() => rawText.value.trim().length >= 20 && !busy.value)

function addExclude() {
  const v = excludeInput.value.trim()
  if (v && !excludedTopics.value.includes(v)) excludedTopics.value.push(v)
  excludeInput.value = ''
}
function removeExclude(i: number) {
  excludedTopics.value.splice(i, 1)
}

function onExcel(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) excelFile.value = f
  ;(e.target as HTMLInputElement).value = ''
}

async function pollActive(run: Run) {
  phase.value = run.status === 'done' ? 'report' : run.status === 'failed' ? 'error' : 'progress'
  if (run.status === 'done' || run.status === 'failed') return
  const final = await store.pollRun(run.id, (r) => {
    if (!cancelled) activeRun.value = r
  })
  if (cancelled) return
  activeRun.value = final
  if (final.status === 'done') {
    phase.value = 'report'
    toast.success('Прогон готов')
  } else {
    phase.value = 'error'
  }
}

async function generate() {
  if (!canSubmit.value) return
  busy.value = true
  cancelled = false
  try {
    const rankingWeights = {
      evidence: weights.evidence / 100,
      feasibility: weights.feasibility / 100,
      impact: weights.impact / 100,
      novelty: weights.novelty / 100,
      risk: weights.risk / 100,
    }
    const topics = excludedTopics.value.length ? excludedTopics.value : undefined

    let run: Run
    if (excelFile.value) {
      // На Excel-пути числа берутся из файла; веса и исключения передаём отдельно.
      run = await store.createRunFromExcel(excelFile.value, rawText.value.trim(), {
        language: 'ru',
        rankingWeights,
        excludedTopics: topics,
      })
    } else {
      const body: CreateRunRequest = {
        rawText: rawText.value.trim(),
        language: 'ru',
        excludedTopics: topics,
        rankingWeights,
      }
      run = await store.createRun(body)
    }
    activeRun.value = run
    await pollActive(run)
  } catch (e) {
    toast.error('Не удалось запустить прогон', { description: (e as Error).message })
    phase.value = 'empty'
  } finally {
    busy.value = false
  }
}

// Открытие существующего прогона по :runId.
async function openRun(id: string) {
  cancelled = false
  busy.value = true
  phase.value = 'progress'
  try {
    await store.fetchRun(id)
    const run = store.current
    if (!run) {
      phase.value = 'error'
      return
    }
    activeRun.value = run
    await pollActive(run)
  } catch (e) {
    toast.error('Не удалось открыть прогон', { description: (e as Error).message })
    phase.value = 'error'
  } finally {
    busy.value = false
  }
}

watch(
  () => props.runId,
  (id) => {
    cancelled = true
    if (id) {
      cancelled = false
      openRun(id)
    } else {
      activeRun.value = null
      phase.value = 'empty'
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  cancelled = true
})
</script>

<template>
  <section>
    <div class="mb-6 flex items-end gap-4">
      <div>
        <h1 class="h1 mb-1 text-[26px] font-extrabold text-ink">Генератор гипотез</h1>
        <p class="text-[14px] text-sec">Опишите цель — система подберёт источники и предложит гипотезы</p>
      </div>
      <div class="flex-1"></div>
      <button
        class="btn btn-primary px-5 py-2.5 text-[14px]"
        :disabled="!canSubmit"
        @click="generate"
      >
        <Icon :icon="busy ? 'lucide:loader-circle' : 'lucide:play'" class="size-4" :class="{ 'animate-spin': busy }" />
        {{ busy ? 'Запуск…' : 'Сгенерировать' }}
      </button>
    </div>

    <div class="grid grid-cols-1 items-start gap-5 lg:grid-cols-[380px_1fr]">
      <!-- ЛЕВАЯ КАРТОЧКА — ФОРМА -->
      <div class="card p-6">
        <div class="lbl mb-2">Целевое свойство / проблема</div>
        <textarea
          v-model="rawText"
          rows="5"
          class="inp mb-1.5"
          placeholder="Снизить потери Ni в породных хвостах КГМК в классе +71 мкм, без остановки текущей схемы"
        />
        <p class="mb-5 text-[11.5px]" :class="rawText.trim().length >= 20 ? 'text-faint' : 'text-warn'">
          Опишите цель и ограничения свободным текстом (минимум 20 символов).
        </p>

        <!-- Excel «хвосты» (опционально) -->
        <div class="lbl mb-2">Данные задачи (Excel «хвосты»)</div>
        <div v-if="excelFile" class="mb-2 flex items-center gap-2.5 rounded-[10px] bg-muted px-2.5 py-2">
          <Icon icon="lucide:file-spreadsheet" class="size-4 text-ok" />
          <span class="flex-1 truncate text-[12.5px] text-body">{{ excelFile.name }}</span>
          <button class="text-faint hover:text-danger" @click="excelFile = null">
            <Icon icon="lucide:x" class="size-3.5" />
          </button>
        </div>
        <label
          v-else
          class="mb-1.5 flex w-full cursor-pointer items-center justify-center gap-2 rounded-[10px] border border-dashed border-bds py-2.5 text-[12.5px] text-sec transition hover:border-ink hover:text-ink"
        >
          <Icon icon="lucide:plus" class="size-[15px]" />
          Прикрепить .xlsx
          <input type="file" accept=".xlsx,.xls" class="hidden" @change="onExcel" />
        </label>
        <p class="mb-5 text-[11px] text-faint">
          Только «профиль хвостов» организаторов. Числа берутся из файла, текст — качественный контекст.
        </p>

        <!-- Источники (read-only) -->
        <div class="lbl mb-2">Источники</div>
        <div class="mb-5 flex items-start gap-2 rounded-[10px] border border-bd px-3 py-2.5 text-[12.5px] text-sec">
          <Icon icon="lucide:library" class="mt-0.5 size-4 shrink-0 text-faint" />
          Используется общая база знаний (доклады · патенты · статьи). Загрузка литературы —
          в разделе «База знаний».
        </div>

        <div class="h-px bg-bd"></div>

        <!-- Продвинутые настройки -->
        <button
          type="button"
          class="flex w-full items-center gap-2 py-2.5 text-[13px] font-medium text-sec transition hover:text-ink"
          @click="advOpen = !advOpen"
        >
          <Icon
            icon="lucide:chevron-right"
            class="size-3.5 transition-transform"
            :class="{ 'rotate-90': advOpen }"
          />
          Продвинутые настройки
          <span class="ml-auto text-[11px] text-faint">ограничения · исключения · веса</span>
        </button>

        <div v-if="advOpen" class="pt-3">
          <!-- Исключаемые темы (excludedTopics — реальное поле API) -->
          <div class="lbl mb-2">Исключаемые темы</div>
          <div class="mb-2 flex flex-wrap gap-1.5">
            <span
              v-for="(c, i) in excludedTopics"
              :key="i"
              class="inline-flex items-center gap-1 rounded-full border border-bds px-2.5 py-1 text-[11.5px] text-sec"
            >
              {{ c }}
              <button @click="removeExclude(i)"><Icon icon="lucide:x" class="size-3" /></button>
            </span>
          </div>
          <input
            v-model="excludeInput"
            class="inp mb-4 text-[13px]"
            placeholder="тема для исключения — Enter"
            @keydown.enter.prevent="addExclude"
          />

          <!-- Веса -->
          <div class="lbl mb-3">Веса критериев ранжирования</div>
          <div class="space-y-3.5">
            <div v-for="(label, key) in WEIGHT_LABELS" :key="key">
              <div class="mb-1.5 flex justify-between text-[12.5px]">
                <span class="text-body">{{ label }}</span>
                <span class="mono text-ink">{{ weights[key] }}</span>
              </div>
              <input v-model.number="weights[key]" type="range" min="0" max="100" />
            </div>
          </div>
        </div>
      </div>

      <!-- ПРАВАЯ ОБЛАСТЬ — РЕЗУЛЬТАТ -->
      <div class="min-h-[520px]">
        <!-- empty -->
        <div
          v-if="phase === 'empty'"
          class="card flex h-[520px] flex-col items-center justify-center border-dashed text-center"
          style="border-style: dashed"
        >
          <div class="mb-4 grid size-11 place-items-center rounded-full border border-bds">
            <Icon icon="lucide:play" class="size-[18px] text-faint" />
          </div>
          <div class="mb-1.5 text-[15px] font-semibold text-ink">Готово к генерации</div>
          <p class="max-w-[340px] text-[13.5px] text-sec">
            Опишите цель (можно приложить Excel «хвосты») и нажмите «Сгенерировать» —
            гипотезы появятся здесь.
          </p>
        </div>

        <!-- progress -->
        <div v-else-if="phase === 'progress'" class="card p-6">
          <div class="mb-4 flex items-center gap-2.5">
            <span class="size-1.5 rounded-full bg-ink"></span>
            <span class="lbl text-body">Идёт генерация…</span>
            <span class="ml-auto text-[12px] text-faint">≈ 45–90 сек</span>
          </div>
          <StageStepper v-if="activeRun" :status="activeRun.status" />
        </div>

        <!-- error -->
        <div v-else-if="phase === 'error'" class="card flex flex-col items-center gap-3 p-10 text-center">
          <Icon icon="lucide:circle-x" class="size-8 text-danger" />
          <div class="text-[15px] font-semibold text-ink">Прогон не удался</div>
          <p class="max-w-[360px] text-[13px] text-sec">
            {{ activeRun?.error || 'Неизвестная ошибка. Уточните описание задачи и повторите.' }}
          </p>
          <button class="btn btn-secondary px-4 py-2 text-[13px]" @click="generate">Повторить</button>
        </div>

        <!-- report -->
        <RunReport
          v-else-if="phase === 'report' && activeRun"
          :run="activeRun"
          @reranked="activeRun = $event"
        />
      </div>
    </div>
  </section>
</template>

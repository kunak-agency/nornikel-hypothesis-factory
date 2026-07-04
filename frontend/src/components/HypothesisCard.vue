<script setup lang="ts">
import { ref, computed } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import type { Hypothesis, FeedbackVerdict } from '@/api/types'
import { runsApi } from '@/api/runs'
import { parseCriticNotes } from '@/lib/criticNotes'

const props = defineProps<{ hypothesis: Hypothesis; index: number }>()

const critics = computed(() => parseCriticNotes(props.hypothesis.criticNotes))

// Оценки (чипы). riskPenalty — ШТРАФ: ниже — лучше.
const scoreChips = computed(() => {
  const s = props.hypothesis.scores
  if (!s) return []
  return [
    { key: 'total', label: 'Итог', value: s.total, strong: true },
    { key: 'evidence', label: 'Доказ.', value: s.evidenceStrength },
    { key: 'feasibility', label: 'Реализ.', value: s.feasibility },
    { key: 'impact', label: 'Эффект', value: s.impact },
    { key: 'novelty', label: 'Новизна', value: s.novelty },
    { key: 'confidence', label: 'Увер.', value: s.confidence },
    { key: 'risk', label: 'Риск-штраф', value: s.riskPenalty, penalty: true },
  ]
})

const kpi = computed(() => props.hypothesis.expectedKpiEffect)
const kpiDown = computed(() => kpi.value?.direction === 'decrease')

const CRITIC_ICON: Record<string, string> = {
  технолог: 'lucide:wrench',
  экономист: 'lucide:coins',
  'рецензент новизны': 'lucide:sparkles',
}

// --- Обратная связь ---
const verdict = ref<FeedbackVerdict | null>(null)
const comment = ref('')
const submitting = ref(false)
const submitted = ref<FeedbackVerdict | null>(null)

const VERDICTS: { key: FeedbackVerdict; label: string; icon: string; cls: string }[] = [
  { key: 'confirmed', label: 'Подтвердить', icon: 'lucide:check', cls: 'text-ok' },
  { key: 'rejected', label: 'Отклонить', icon: 'lucide:x', cls: 'text-danger' },
  { key: 'needs_revision', label: 'На доработку', icon: 'lucide:pencil', cls: 'text-warn' },
]

function fmt(n: number) {
  return typeof n === 'number' ? n.toFixed(2).replace(/\.00$/, '') : '—'
}

async function sendFeedback() {
  if (!verdict.value) return
  submitting.value = true
  try {
    await runsApi.submitFeedback(props.hypothesis.id, {
      verdict: verdict.value,
      comment: comment.value || undefined,
    })
    submitted.value = verdict.value
    toast.success('Оценка принята', {
      description: 'Она повлияет на будущие генерации, не на текущий отчёт.',
    })
    comment.value = ''
    verdict.value = null
  } catch (e) {
    toast.error('Не удалось отправить оценку', { description: (e as Error).message })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="card p-5">
    <!-- Заголовок + эффект на KPI -->
    <div class="flex items-start gap-4">
      <span class="mono pt-0.5 text-[12px] text-accent">#{{ index + 1 }}</span>
      <div class="min-w-0 flex-1">
        <div class="h3 text-[16.5px] font-bold leading-snug text-ink">
          {{ hypothesis.statement }}
        </div>
      </div>
      <div v-if="kpi" class="shrink-0 text-right">
        <div
          class="mono inline-flex items-center gap-1 text-[17px] font-semibold"
          :class="kpiDown ? 'text-ok' : 'text-ink'"
        >
          <Icon :icon="kpiDown ? 'lucide:arrow-down' : 'lucide:arrow-up'" class="size-4" />
          {{ kpi.magnitude }}
        </div>
        <div class="lbl mt-0.5">{{ kpi.metric }}</div>
      </div>
    </div>

    <!-- Механизм -->
    <p v-if="hypothesis.mechanism" class="mt-3 text-[13.5px] leading-relaxed text-sec">
      {{ hypothesis.mechanism }}
    </p>

    <!-- Новизна -->
    <p v-if="hypothesis.noveltyReason" class="mt-2 text-[12.5px] leading-relaxed text-faint">
      <span class="font-semibold text-sec">Новизна:</span> {{ hypothesis.noveltyReason }}
    </p>

    <!-- Риски -->
    <div v-if="hypothesis.risks?.length" class="mt-3 flex flex-wrap gap-1.5">
      <span
        v-for="(risk, i) in hypothesis.risks"
        :key="i"
        class="rounded-full bg-warn/10 px-2.5 py-1 text-[11.5px] text-warn"
      >
        {{ risk }}
      </span>
    </div>

    <!-- Оценки -->
    <div v-if="scoreChips.length" class="mt-4 flex flex-wrap gap-1.5">
      <span
        v-for="c in scoreChips"
        :key="c.key"
        class="mono inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[11.5px]"
        :class="c.strong ? 'border-ink/20 bg-ink/5 text-ink' : 'border-bd text-sec'"
        :title="c.penalty ? 'Штраф за риск: ниже — лучше' : undefined"
      >
        {{ c.label }} <b class="font-semibold">{{ fmt(c.value) }}</b>
      </span>
    </div>

    <div class="my-4 h-px bg-bd"></div>

    <!-- Отзывы ИИ (0–3 судьи) -->
    <div v-if="critics.length" class="space-y-2">
      <div class="lbl">Отзывы ИИ</div>
      <div
        v-for="(note, i) in critics"
        :key="i"
        class="rounded-[10px] bg-muted px-3 py-2.5"
      >
        <div
          v-if="note.role"
          class="mb-1 flex items-center gap-1.5 text-[11.5px] font-semibold text-body"
        >
          <Icon :icon="CRITIC_ICON[note.role] || 'lucide:message-square'" class="size-3.5" />
          {{ note.role }}
        </div>
        <p class="text-[12.5px] leading-relaxed text-sec">{{ note.text }}</p>
      </div>
    </div>

    <!-- Дорожная карта проверки (только чтение — save-эндпоинта нет) -->
    <div v-if="hypothesis.verificationPlan?.length" class="mt-4">
      <div class="lbl mb-2 flex items-center gap-1.5">
        Дорожная карта проверки
        <span class="rounded bg-bd px-1.5 py-0.5 text-[10px] font-medium text-faint">только чтение</span>
      </div>
      <div class="flex flex-col gap-2">
        <div
          v-for="(st, i) in hypothesis.verificationPlan"
          :key="i"
          class="rounded-[10px] border border-bd p-3"
        >
          <div class="flex items-start gap-2.5">
            <span
              class="mono grid size-5 shrink-0 place-items-center rounded-full bg-ink text-[11px] text-white"
            >
              {{ i + 1 }}
            </span>
            <div class="min-w-0 flex-1">
              <div class="text-[13px] font-medium text-body">{{ st.step }}</div>
              <div v-if="st.resource" class="mt-0.5 text-[11.5px] text-faint">{{ st.resource }}</div>
              <div v-if="st.successCriterion" class="mt-1 flex items-center gap-1 text-[11.5px] text-sec">
                <Icon icon="lucide:target" class="size-3.5 text-accent" />
                {{ st.successCriterion }}
              </div>
              <div class="mt-1.5 flex flex-wrap gap-3 text-[11px] text-faint">
                <span v-if="st.estimatedDuration" class="mono inline-flex items-center gap-1">
                  <Icon icon="lucide:clock" class="size-3.5" />{{ st.estimatedDuration }}
                </span>
                <span v-if="st.estimatedCost" class="mono inline-flex items-center gap-1">
                  <Icon icon="lucide:coins" class="size-3.5" />{{ st.estimatedCost }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Источники (evidenceRefs → claims; дословные цитаты — во вкладке Evidence) -->
    <div
      v-if="hypothesis.evidenceRefs?.length"
      class="mt-3 flex items-center gap-1.5 text-[12px] text-faint"
      title="Дословные цитаты — во вкладке Evidence"
    >
      <Icon icon="lucide:link" class="size-3.5" />
      Опирается на {{ hypothesis.evidenceRefs.length }} факт(ов) — см. вкладку Evidence
    </div>

    <div class="my-4 h-px bg-bd"></div>

    <!-- Обратная связь -->
    <div v-if="submitted" class="flex items-center gap-2 text-[13px] text-ok">
      <Icon icon="lucide:check-circle-2" class="size-4" />
      Оценка отправлена — повлияет на будущие прогоны
    </div>
    <div v-else>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="v in VERDICTS"
          :key="v.key"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-[10px] border px-3 py-1.5 text-[12.5px] font-medium transition"
          :class="
            verdict === v.key
              ? 'border-ink bg-muted ' + v.cls
              : 'border-bd text-sec hover:border-bds'
          "
          @click="verdict = v.key"
        >
          <Icon :icon="v.icon" class="size-3.5" />
          {{ v.label }}
        </button>
      </div>
      <div v-if="verdict" class="mt-2.5 flex items-start gap-2">
        <textarea
          v-model="comment"
          rows="2"
          class="inp text-[13px]"
          placeholder="Комментарий (необязательно)"
        />
        <button
          class="btn btn-primary shrink-0 px-4 py-2 text-[13px]"
          :disabled="submitting"
          @click="sendFeedback"
        >
          {{ submitting ? '…' : 'Оценить' }}
        </button>
      </div>
    </div>
  </div>
</template>

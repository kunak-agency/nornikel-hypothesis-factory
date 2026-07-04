<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import type { Run, Claim } from '@/api/types'
import { runsApi } from '@/api/runs'
import HypothesisCard from '@/components/HypothesisCard.vue'

const props = defineProps<{ run: Run }>()
const emit = defineEmits<{ reranked: [Run] }>()

const spec = computed(() => props.run.problemSpec)
const hypotheses = computed(() =>
  [...(props.run.hypotheses ?? [])].sort((a, b) => (a.rank ?? 0) - (b.rank ?? 0)),
)

type Tab = 'hypotheses' | 'evidence' | 'graph' | 'weights'
const tab = ref<Tab>('hypotheses')

// Экспорт — реальные эндпоинты, открываются напрямую.
const EXPORTS = computed(() => [
  { label: 'Markdown', icon: 'lucide:file-text', href: runsApi.reportUrl(props.run.id, 'md') },
  { label: 'PDF', icon: 'lucide:file-down', href: runsApi.reportUrl(props.run.id, 'pdf') },
  { label: 'DOCX', icon: 'lucide:file-type', href: runsApi.reportUrl(props.run.id, 'docx') },
  { label: 'CSV', icon: 'lucide:table', href: runsApi.reportUrl(props.run.id, 'csv') },
  { label: 'Jira', icon: 'lucide:list-checks', href: runsApi.jiraUrl(props.run.id) },
])

// --- Evidence (GET /v1/runs/:id/claims) ---
const claims = ref<Claim[]>([])
const claimsLoaded = ref(false)
const claimsLoading = ref(false)
const claimsError = ref<string | null>(null)

async function loadClaims() {
  tab.value = 'evidence'
  if (claimsLoaded.value || claimsLoading.value) return
  claimsLoading.value = true
  claimsError.value = null
  try {
    const resp = await runsApi.claims(props.run.id)
    claims.value = resp.items ?? []
    claimsLoaded.value = true
  } catch (e) {
    claimsError.value = (e as Error).message
  } finally {
    claimsLoading.value = false
  }
}

// --- Граф (GET /v1/runs/:id/graph) ---
interface GraphNode {
  id: string
  type: string
  label?: string
}
const graphLoaded = ref(false)
const graphLoading = ref(false)
const graphNodes = ref<GraphNode[]>([])
const graphEdgeCount = ref(0)
const graphError = ref<string | null>(null)

async function loadGraph() {
  tab.value = 'graph'
  if (graphLoaded.value || graphLoading.value) return
  graphLoading.value = true
  graphError.value = null
  try {
    const data = await runsApi.graph(props.run.id)
    graphNodes.value = data?.nodes ?? []
    graphEdgeCount.value = (data?.edges ?? []).length
    graphLoaded.value = true
  } catch (e) {
    graphError.value = (e as Error).message
  } finally {
    graphLoading.value = false
  }
}
const graphByType = computed(() => {
  const groups: Record<string, number> = {}
  for (const n of graphNodes.value) groups[n.type] = (groups[n.type] ?? 0) + 1
  return groups
})
const TYPE_RU: Record<string, string> = {
  entity: 'сущности',
  claim: 'факты (claims)',
  hypothesis: 'гипотезы',
}

// --- Веса (POST /v1/runs/:id/rerank) ---
const weights = reactive({ evidence: 50, feasibility: 50, impact: 50, novelty: 50, risk: 50 })
const WEIGHT_LABELS: Record<keyof typeof weights, string> = {
  evidence: 'Доказательность',
  feasibility: 'Реализуемость',
  impact: 'Эффект',
  novelty: 'Новизна',
  risk: 'Штраф за риск',
}
const reranking = ref(false)

async function rerank() {
  reranking.value = true
  try {
    const updated = await runsApi.rerank(props.run.id, {
      evidence: weights.evidence / 100,
      feasibility: weights.feasibility / 100,
      impact: weights.impact / 100,
      novelty: weights.novelty / 100,
      risk: weights.risk / 100,
    })
    emit('reranked', updated)
    tab.value = 'hypotheses'
    toast.success('Порядок пересчитан', {
      description: 'Новый порядок сохранён и попадёт в экспорты.',
    })
  } catch (e) {
    toast.error('Не удалось пересчитать', { description: (e as Error).message })
  } finally {
    reranking.value = false
  }
}

const TABS: { key: Tab; label: string; onClick?: () => void }[] = [
  { key: 'hypotheses', label: 'Гипотезы' },
  { key: 'evidence', label: 'Evidence', onClick: loadClaims },
  { key: 'graph', label: 'Граф', onClick: loadGraph },
  { key: 'weights', label: 'Веса' },
]
function selectTab(t: (typeof TABS)[number]) {
  if (t.onClick) t.onClick()
  else tab.value = t.key
}

function confPct(c: string) {
  const n = parseFloat(c)
  return isNaN(n) ? c : `${Math.round((n <= 1 ? n * 100 : n))}%`
}
</script>

<template>
  <div class="space-y-4">
    <!-- Шапка контекста -->
    <div class="card p-5">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[13px]">
        <span v-if="spec?.targetKpi" class="font-semibold text-ink">{{ spec.targetKpi }}</span>
        <span v-if="spec?.plant" class="text-sec">Фабрика: <b class="text-body">{{ spec.plant }}</b></span>
        <span v-if="spec?.targetMetals?.length" class="text-sec">
          Металлы: <b class="mono text-body">{{ spec.targetMetals.join(', ') }}</b>
        </span>
      </div>
      <div v-if="spec?.lossHotspots?.length" class="mt-1.5 text-[12.5px] text-sec">
        Точки потерь: {{ spec.lossHotspots.join(' · ') }}
      </div>
      <div v-if="spec?.constraints?.length" class="mt-2.5 flex flex-wrap gap-1.5">
        <span
          v-for="(c, i) in spec.constraints"
          :key="i"
          class="rounded-full bg-accentbg px-2.5 py-1 text-[11.5px] text-accent"
        >
          {{ c }}
        </span>
      </div>
    </div>

    <!-- Пробелы в знаниях -->
    <div v-if="run.knowledgeGaps?.length" class="card border-warn/30 bg-warn/5 p-4">
      <div class="flex items-center gap-2 text-[13px] font-semibold text-warn">
        <Icon icon="lucide:triangle-alert" class="size-4" />
        Пробелы в знаниях
      </div>
      <p class="mt-1 text-[12.5px] leading-relaxed text-sec">
        Слабая опора в базе по точкам: {{ run.knowledgeGaps.join(' · ') }}. Это индикатор
        надёжности, а не ошибка.
      </p>
    </div>

    <!-- Панель экспорта -->
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-[14px] font-semibold text-ink">{{ hypotheses.length }} гипотез</span>
      <span class="text-[12.5px] text-faint">ранжировано</span>
      <div class="flex-1"></div>
      <a
        v-for="ex in EXPORTS"
        :key="ex.label"
        :href="ex.href"
        target="_blank"
        rel="noopener"
        class="btn btn-secondary px-3 py-1.5 text-[12.5px]"
      >
        <Icon :icon="ex.icon" class="size-3.5" />
        {{ ex.label }}
      </a>
    </div>

    <!-- Вкладки -->
    <div class="flex items-center gap-1 border-b border-bd">
      <button
        v-for="t in TABS"
        :key="t.key"
        type="button"
        class="-mb-px border-b-2 px-3 py-2 text-[13.5px] font-medium transition"
        :class="tab === t.key ? 'border-ink text-ink' : 'border-transparent text-faint hover:text-sec'"
        @click="selectTab(t)"
      >
        {{ t.label }}
      </button>
    </div>

    <!-- Гипотезы -->
    <div v-show="tab === 'hypotheses'" class="flex flex-col gap-3">
      <HypothesisCard v-for="(h, i) in hypotheses" :key="h.id" :hypothesis="h" :index="i" />
      <div v-if="!hypotheses.length" class="card p-8 text-center text-[13px] text-sec">
        Гипотез нет. Возможно, задача вне поддерживаемого домена (флотация) — уточните
        описание и запустите заново.
      </div>
    </div>

    <!-- Evidence (claims) -->
    <div v-show="tab === 'evidence'">
      <div v-if="claimsLoading" class="card flex items-center gap-2 p-5 text-[13px] text-sec">
        <Icon icon="lucide:loader-circle" class="size-4 animate-spin" /> Загрузка фактов…
      </div>
      <div v-else-if="claimsError" class="card p-5 text-[13px] text-danger">
        Ошибка: {{ claimsError }}
      </div>
      <div v-else-if="!claims.length" class="card p-8 text-center text-[13px] text-sec">
        Для этого прогона нет извлечённых фактов.
      </div>
      <div v-else class="flex flex-col gap-2.5">
        <div v-for="c in claims" :key="c.id" class="card p-4">
          <div class="flex items-start gap-3">
            <Icon icon="lucide:quote" class="mt-0.5 size-4 shrink-0 text-faint" />
            <div class="min-w-0 flex-1">
              <p class="text-[13.5px] leading-relaxed text-body">{{ c.quote }}</p>
              <div class="mt-2 flex flex-wrap items-center gap-2 text-[11.5px]">
                <span v-if="c.subject" class="rounded-md bg-muted px-2 py-0.5 text-sec">
                  {{ c.subject }}
                </span>
                <span
                  v-if="c.metric"
                  class="mono inline-flex items-center gap-1 text-sec"
                >
                  <Icon
                    :icon="c.effectDirection === 'decrease' ? 'lucide:arrow-down' : 'lucide:arrow-up'"
                    class="size-3.5"
                  />
                  {{ c.metric }} {{ c.effectMagnitude }}
                </span>
                <span v-if="c.sourceConfidence" class="text-faint">
                  уверенность: {{ confPct(c.sourceConfidence) }}
                </span>
                <span v-if="c.citedByHypothesisIds?.length" class="text-faint">
                  · цитируется в {{ c.citedByHypothesisIds.length }} гип.
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Граф -->
    <div v-show="tab === 'graph'" class="card p-5">
      <div v-if="graphLoading" class="flex items-center gap-2 text-[13px] text-sec">
        <Icon icon="lucide:loader-circle" class="size-4 animate-spin" /> Загрузка графа…
      </div>
      <div v-else-if="graphError" class="text-[13px] text-danger">Ошибка графа: {{ graphError }}</div>
      <div v-else>
        <div class="mb-3 text-[13px] text-sec">
          Связи <b class="text-body">источник → claim → гипотеза</b>:
          {{ graphNodes.length }} узлов, {{ graphEdgeCount }} связей.
        </div>
        <div class="flex flex-wrap gap-2">
          <span
            v-for="(count, type) in graphByType"
            :key="type"
            class="rounded-[10px] border border-bd px-3 py-2 text-[12.5px] text-body"
          >
            <b class="mono">{{ count }}</b> {{ TYPE_RU[type] || type }}
          </span>
        </div>
      </div>
    </div>

    <!-- Веса (rerank) -->
    <div v-show="tab === 'weights'" class="card p-5">
      <div class="mb-3 flex items-start gap-2 rounded-[10px] bg-warn/5 p-3 text-[12px] text-warn">
        <Icon icon="lucide:triangle-alert" class="mt-0.5 size-3.5 shrink-0" />
        Пересчёт перезаписывает сохранённый порядок для всего прогона (влияет и на экспорты,
        и на список «Исследования») — это не песочница.
      </div>
      <div class="space-y-3.5">
        <div v-for="(label, key) in WEIGHT_LABELS" :key="key">
          <div class="mb-1.5 flex justify-between text-[12.5px]">
            <span class="text-body">{{ label }}</span>
            <span class="mono text-ink">{{ weights[key] }}</span>
          </div>
          <input v-model.number="weights[key]" type="range" min="0" max="100" />
        </div>
      </div>
      <button
        class="btn btn-primary mt-4 w-full py-2.5 text-[13.5px]"
        :disabled="reranking"
        @click="rerank"
      >
        <Icon
          :icon="reranking ? 'lucide:loader-circle' : 'lucide:arrow-down-up'"
          class="size-4"
          :class="{ 'animate-spin': reranking }"
        />
        {{ reranking ? 'Пересчёт…' : 'Пересчитать порядок' }}
      </button>
    </div>
  </div>
</template>

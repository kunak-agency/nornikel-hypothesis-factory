<script setup lang="ts">
import { ref, computed } from 'vue'
import { Icon } from '@iconify/vue'

import type { Run } from '@/api/types'
import { runsApi } from '@/api/runs'
import HypothesisCard from '@/components/HypothesisCard.vue'

const props = defineProps<{ run: Run }>()

const spec = computed(() => props.run.problemSpec)
const hypotheses = computed(() =>
  [...(props.run.hypotheses ?? [])].sort((a, b) => (a.rank ?? 0) - (b.rank ?? 0)),
)

type Tab = 'hypotheses' | 'graph'
const tab = ref<Tab>('hypotheses')

// Экспорт — реальные эндпоинты, открываются напрямую.
const EXPORTS = computed(() => [
  { label: 'Markdown', icon: 'lucide:file-text', href: runsApi.reportUrl(props.run.id, 'md') },
  { label: 'PDF', icon: 'lucide:file-down', href: runsApi.reportUrl(props.run.id, 'pdf') },
  { label: 'DOCX', icon: 'lucide:file-type', href: runsApi.reportUrl(props.run.id, 'docx') },
  { label: 'CSV', icon: 'lucide:table', href: runsApi.reportUrl(props.run.id, 'csv') },
  { label: 'Jira', icon: 'lucide:list-checks', href: runsApi.jiraUrl(props.run.id) },
])

// --- Граф (реальный /graph, компактная сводка) ---
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
    <div
      v-if="run.knowledgeGaps?.length"
      class="card border-warn/30 bg-warn/5 p-4"
    >
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
        type="button"
        class="-mb-px border-b-2 px-3 py-2 text-[13.5px] font-medium transition"
        :class="tab === 'hypotheses' ? 'border-ink text-ink' : 'border-transparent text-faint hover:text-sec'"
        @click="tab = 'hypotheses'"
      >
        Гипотезы
      </button>
      <button
        type="button"
        class="-mb-px border-b-2 px-3 py-2 text-[13.5px] font-medium transition"
        :class="tab === 'graph' ? 'border-ink text-ink' : 'border-transparent text-faint hover:text-sec'"
        @click="loadGraph"
      >
        Граф
      </button>
    </div>

    <!-- Гипотезы -->
    <div v-show="tab === 'hypotheses'" class="flex flex-col gap-3">
      <HypothesisCard
        v-for="(h, i) in hypotheses"
        :key="h.id"
        :hypothesis="h"
        :index="i"
      />
      <div v-if="!hypotheses.length" class="card p-8 text-center text-[13px] text-sec">
        Гипотез нет. Возможно, задача вне поддерживаемого домена (флотация) — уточните
        описание и запустите заново.
      </div>
    </div>

    <!-- Граф: источник → claim → гипотеза -->
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'

import type { RunStatus } from '@/api/types'
import { RUN_STAGES, STATUS_LABELS } from '@/api/runs'

// Вертикальный стептер стадий прогона: pending → … → critiquing → done.
const props = defineProps<{ status: RunStatus }>()

const activeIndex = computed(() => {
  if (props.status === 'done') return RUN_STAGES.length
  if (props.status === 'failed') return RUN_STAGES.indexOf('generating')
  return RUN_STAGES.indexOf(props.status)
})

const running = computed(() => props.status !== 'done' && props.status !== 'failed')

function state(i: number) {
  if (i < activeIndex.value) return 'done'
  if (i === activeIndex.value && running.value) return 'active'
  return 'todo'
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div v-for="(stage, i) in RUN_STAGES" :key="stage" class="flex items-center gap-3 py-1.5">
      <span
        class="grid size-[22px] shrink-0 place-items-center rounded-full border-[1.5px]"
        :class="{
          'border-ink bg-ink': state(i) === 'done',
          'border-accent': state(i) === 'active',
          'border-bds': state(i) === 'todo',
        }"
      >
        <Icon v-if="state(i) === 'done'" icon="lucide:check" class="size-[11px] text-white" />
        <Icon
          v-else-if="state(i) === 'active'"
          icon="lucide:loader-circle"
          class="size-3 animate-spin text-accent"
        />
      </span>
      <span
        class="text-[13.5px]"
        :class="state(i) === 'todo' ? 'text-faint' : 'text-ink'"
      >
        {{ STATUS_LABELS[stage] }}
      </span>
    </div>
  </div>
</template>

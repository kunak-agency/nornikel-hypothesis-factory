<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'

import type { RunStatus } from '@/api/types'
import { STATUS_LABELS } from '@/api/runs'

const props = defineProps<{ status: RunStatus }>()

// Внешний вид бейджа по статусу: done — зелёный, failed — красный,
// промежуточные — синий с «дышащей» точкой.
const meta = computed(() => {
  switch (props.status) {
    case 'done':
      return { dot: 'bg-ok', text: 'text-ok', bg: 'bg-ok/10', icon: 'lucide:check', spin: false }
    case 'failed':
      return {
        dot: 'bg-danger',
        text: 'text-danger',
        bg: 'bg-danger/10',
        icon: 'lucide:x',
        spin: false,
      }
    default:
      return {
        dot: 'bg-accent',
        text: 'text-accent',
        bg: 'bg-accent/10',
        icon: 'lucide:loader-circle',
        spin: true,
      }
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[12px] font-medium"
    :class="[meta.bg, meta.text]"
  >
    <Icon
      v-if="meta.spin"
      icon="lucide:loader-circle"
      class="size-3 animate-spin"
    />
    <span v-else class="size-1.5 rounded-full" :class="meta.dot"></span>
    {{ STATUS_LABELS[status] }}
  </span>
</template>

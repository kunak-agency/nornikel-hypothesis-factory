<script setup lang="ts">
import {
  SelectContent,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'
import { Icon } from '@iconify/vue'

interface Option {
  value: string
  label: string
}

defineProps<{ options: Option[]; placeholder?: string }>()
// v-model через reka-ui SelectRoot.
const model = defineModel<string>()
</script>

<template>
  <SelectRoot v-model="model">
    <SelectTrigger
      class="flex w-full items-center justify-between gap-2 rounded-md border border-border bg-bg px-3 py-2 text-sm text-text outline-none focus:ring-2 focus:ring-brand/40"
    >
      <SelectValue :placeholder="placeholder ?? 'Выберите…'" />
      <Icon icon="lucide:chevron-down" class="size-4 text-muted" />
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        position="popper"
        :side-offset="4"
        class="z-50 min-w-(--reka-select-trigger-width) overflow-hidden rounded-md border border-border bg-surface shadow-lg"
      >
        <SelectViewport class="p-1">
          <SelectItem
            v-for="opt in options"
            :key="opt.value"
            :value="opt.value"
            class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-text outline-none data-[highlighted]:bg-brand/10 data-[highlighted]:text-brand"
          >
            <SelectItemText>{{ opt.label }}</SelectItemText>
            <SelectItemIndicator>
              <Icon icon="lucide:check" class="size-4 text-brand" />
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

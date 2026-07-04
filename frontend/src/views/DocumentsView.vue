<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { documentsApi } from '@/api/documents'
import type { DocumentItem } from '@/api/types'

const items = ref<DocumentItem[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = null
  try {
    const resp = await documentsApi.list()
    items.value = resp.items
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function remove(doc: DocumentItem) {
  try {
    await documentsApi.remove(doc.id)
    items.value = items.value.filter((d) => d.id !== doc.id)
    toast.success('Документ удалён', { description: doc.title })
  } catch (e) {
    toast.error('Не удалось удалить', { description: (e as Error).message })
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-5">
    <h1 class="text-2xl font-bold">База знаний</h1>

    <p v-if="loading" class="text-muted">Загрузка…</p>
    <p v-else-if="error" class="text-muted">Ошибка: {{ error }}</p>
    <p v-else-if="!items.length" class="text-muted">Документов пока нет.</p>

    <ul v-else class="space-y-3">
      <li
        v-for="doc in items"
        :key="doc.id"
        class="flex items-start justify-between gap-4 rounded-card border border-border bg-surface p-4"
      >
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-semibold text-text">{{ doc.title }}</span>
            <span class="rounded-full bg-border px-2 py-0.5 text-xs text-muted">
              {{ doc.sourceType }}
            </span>
            <span class="rounded-full bg-border px-2 py-0.5 text-xs text-muted">
              {{ doc.domain }}
            </span>
          </div>
          <p class="mt-1 text-sm text-muted">
            {{ doc.chunkCount }} чанков · {{ doc.language }}
          </p>
        </div>

        <button
          type="button"
          class="flex size-8 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-red-500/10 hover:text-red-500"
          aria-label="Удалить документ"
          @click="remove(doc)"
        >
          <Icon icon="lucide:trash-2" class="size-4" />
        </button>
      </li>
    </ul>
  </section>
</template>

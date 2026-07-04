<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { documentsApi } from '@/api/documents'
import type { DocumentItem } from '@/api/types'

type Tab = 'documents' | 'equipment'
const tab = ref<Tab>('documents')

const docs = ref<DocumentItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<string | null>(null)

const pendingDelete = ref<DocumentItem | null>(null)
const deleting = ref(false)

async function load() {
  loading.value = true
  error.value = null
  try {
    const resp = await documentsApi.list()
    docs.value = resp.items ?? []
    total.value = resp.total ?? docs.value.length
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function confirmDelete() {
  if (!pendingDelete.value) return
  deleting.value = true
  try {
    await documentsApi.remove(pendingDelete.value.id)
    docs.value = docs.value.filter((d) => d.id !== pendingDelete.value!.id)
    total.value = Math.max(0, total.value - 1)
    toast.success('Документ удалён')
    pendingDelete.value = null
  } catch (e) {
    toast.error('Не удалось удалить', { description: (e as Error).message })
  } finally {
    deleting.value = false
  }
}

const isEmpty = computed(() => !loading.value && !error.value && docs.value.length === 0)

function fmtDate(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

onMounted(load)
</script>

<template>
  <section>
    <div class="mb-6 flex items-end gap-4">
      <div>
        <h1 class="h1 mb-1 text-[26px] font-extrabold text-ink">База знаний</h1>
        <p class="text-[14px] text-sec">Входные материалы, на которых система строит гипотезы</p>
      </div>
      <div class="flex-1"></div>
      <!-- Загрузка в демо недоступна (нужен GPU-профиль ingestion) -->
      <button
        class="btn btn-secondary cursor-not-allowed px-4 py-2.5 text-[13px] opacity-60"
        disabled
        title="Требуется GPU-профиль ingestion — в демо недоступно"
      >
        <Icon icon="lucide:lock" class="size-4" />
        Загрузить документ
      </button>
    </div>

    <!-- Вкладки -->
    <div class="mb-5 flex items-center gap-1 border-b border-bd">
      <button
        type="button"
        class="-mb-px border-b-2 px-3 py-2 text-[13.5px] font-medium transition"
        :class="tab === 'documents' ? 'border-ink text-ink' : 'border-transparent text-faint hover:text-sec'"
        @click="tab = 'documents'"
      >
        Документы
      </button>
      <button
        type="button"
        class="-mb-px border-b-2 px-3 py-2 text-[13.5px] font-medium transition"
        :class="tab === 'equipment' ? 'border-ink text-ink' : 'border-transparent text-faint hover:text-sec'"
        @click="tab = 'equipment'"
      >
        Оборудование
      </button>
    </div>

    <!-- ДОКУМЕНТЫ -->
    <template v-if="tab === 'documents'">
      <div v-if="loading && !docs.length" class="flex flex-col gap-2">
        <div v-for="i in 5" :key="i" class="card h-[54px] animate-pulse bg-muted/40"></div>
      </div>

      <div v-else-if="error" class="card flex flex-col items-center gap-3 p-10 text-center">
        <Icon icon="lucide:cloud-alert" class="size-8 text-faint" />
        <p class="text-[14px] text-sec">Не удалось загрузить документы: {{ error }}</p>
        <button class="btn btn-secondary px-4 py-2 text-[13px]" @click="load">Повторить</button>
      </div>

      <div v-else-if="isEmpty" class="card p-12 text-center text-[14px] text-sec">
        Корпус пуст.
      </div>

      <div v-else class="card overflow-hidden">
        <div class="grid grid-cols-[1fr_130px_120px_70px_110px_40px] gap-4 border-b border-bd px-5 py-3 lbl">
          <span>Название</span><span>Тип</span><span>Домен</span><span>Язык</span>
          <span>Добавлен</span><span></span>
        </div>
        <div
          v-for="(doc, i) in docs"
          :key="doc.id"
          class="row grid grid-cols-[1fr_130px_120px_70px_110px_40px] items-center gap-4 px-5 py-3"
          :class="{ 'border-t border-bd': i }"
        >
          <div class="min-w-0">
            <div class="truncate text-[13.5px] font-medium text-ink">{{ doc.title }}</div>
            <div class="mono text-[11px] text-faint">{{ doc.chunkCount }} фрагм.</div>
          </div>
          <span class="truncate text-[12.5px] text-sec">{{ doc.sourceType || '—' }}</span>
          <span class="truncate text-[12.5px] text-sec">{{ doc.domain || '—' }}</span>
          <span class="mono text-[12px] text-faint">{{ doc.language || '—' }}</span>
          <span class="mono text-[12px] text-faint">{{ fmtDate(doc.createdAt) }}</span>
          <button
            class="grid size-8 place-items-center rounded-md text-faint transition hover:bg-muted hover:text-danger"
            title="Удалить документ"
            @click="pendingDelete = doc"
          >
            <Icon icon="lucide:trash-2" class="size-4" />
          </button>
        </div>
      </div>

      <p class="mt-3 flex items-center gap-1.5 text-[12px] text-faint">
        <Icon icon="lucide:info" class="size-3.5" />
        Демо-корпус предзагружен. Загрузка новых документов требует GPU-профиля индексации.
      </p>
    </template>

    <!-- ОБОРУДОВАНИЕ — эндпоинтов /plant-equipment нет (заглушка) -->
    <template v-else>
      <div class="card flex flex-col items-center gap-3 p-12 text-center">
        <div class="grid size-11 place-items-center rounded-full border border-bds">
          <Icon icon="lucide:settings-2" class="size-[18px] text-faint" />
        </div>
        <div class="text-[15px] font-semibold text-ink">Справочник оборудования</div>
        <p class="max-w-[420px] text-[13px] text-sec">
          Раздел появится, когда бэкенд отдаст API оборудования фабрик
          (мельницы, гидроциклоны, флотомашины). Сейчас эндпоинты недоступны.
        </p>
      </div>
    </template>

    <!-- Подтверждение удаления документа -->
    <Teleport to="body">
      <div v-if="pendingDelete" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/30" @click="pendingDelete = null"></div>
        <div class="card relative w-full max-w-[420px] p-6">
          <div class="mb-1 text-[16px] font-bold text-ink">Удалить документ?</div>
          <p class="mb-5 text-[13px] leading-relaxed text-sec">
            «{{ pendingDelete.title }}» будет удалён вместе со всеми фрагментами. Действие необратимо.
          </p>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost px-4 py-2 text-[13px]" @click="pendingDelete = null">Отмена</button>
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

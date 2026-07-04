<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { toast } from 'vue-sonner'

import { documentsApi } from '@/api/documents'
import { equipmentApi } from '@/api/equipment'
import type { DocumentItem, PlantEquipment, PlantEquipmentRequest, PlantSummary } from '@/api/types'

type Tab = 'documents' | 'equipment'
const tab = ref<Tab>('documents')

/* ---------------- Документы (чтение + удаление) ---------------- */
const docs = ref<DocumentItem[]>([])
const docsLoading = ref(false)
const docsError = ref<string | null>(null)
const pendingDoc = ref<DocumentItem | null>(null)
const deletingDoc = ref(false)

async function loadDocs() {
  docsLoading.value = true
  docsError.value = null
  try {
    const resp = await documentsApi.list()
    docs.value = resp.items ?? []
  } catch (e) {
    docsError.value = (e as Error).message
  } finally {
    docsLoading.value = false
  }
}
async function confirmDeleteDoc() {
  if (!pendingDoc.value) return
  deletingDoc.value = true
  try {
    await documentsApi.remove(pendingDoc.value.id)
    docs.value = docs.value.filter((d) => d.id !== pendingDoc.value!.id)
    toast.success('Документ удалён')
    pendingDoc.value = null
  } catch (e) {
    toast.error('Не удалось удалить', { description: (e as Error).message })
  } finally {
    deletingDoc.value = false
  }
}
const docsEmpty = computed(() => !docsLoading.value && !docsError.value && docs.value.length === 0)

/* ---------------- Оборудование (полный CRUD) ---------------- */
const equipment = ref<PlantEquipment[]>([])
const plants = ref<PlantSummary[]>([])
const eqLoading = ref(false)
const eqError = ref<string | null>(null)
const eqLoaded = ref(false)

// Группировка по фабрике.
const grouped = computed(() => {
  const map = new Map<string, PlantEquipment[]>()
  for (const e of equipment.value) {
    const arr = map.get(e.plantName) ?? []
    arr.push(e)
    map.set(e.plantName, arr)
  }
  return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))
})

async function loadEquipment() {
  eqLoading.value = true
  eqError.value = null
  try {
    const [eqResp, plantsResp] = await Promise.all([
      equipmentApi.list(),
      equipmentApi.plants().catch(() => ({ items: [] })),
    ])
    equipment.value = eqResp.items ?? []
    plants.value = plantsResp.items ?? []
    eqLoaded.value = true
  } catch (e) {
    eqError.value = (e as Error).message
  } finally {
    eqLoading.value = false
  }
}

// Форма добавления/редактирования.
const formOpen = ref(false)
const editingId = ref<string | null>(null)
const saving = ref(false)
const draft = ref<PlantEquipmentRequest & { aliasesText: string }>({
  plantName: '',
  equipmentType: '',
  model: '',
  circuitPosition: '',
  plantAliases: [],
  aliasesText: '',
})

function openAdd() {
  editingId.value = null
  draft.value = {
    plantName: '',
    equipmentType: '',
    model: '',
    circuitPosition: '',
    plantAliases: [],
    aliasesText: '',
  }
  formOpen.value = true
}
function openEdit(e: PlantEquipment) {
  editingId.value = e.id
  draft.value = {
    plantName: e.plantName,
    equipmentType: e.equipmentType,
    model: e.model,
    circuitPosition: e.circuitPosition,
    plantAliases: e.plantAliases ?? [],
    aliasesText: (e.plantAliases ?? []).join(', '),
  }
  formOpen.value = true
}

const canSave = computed(
  () => draft.value.plantName.trim() && draft.value.equipmentType.trim() && !saving.value,
)

async function save() {
  if (!canSave.value) return
  saving.value = true
  const body: PlantEquipmentRequest = {
    plantName: draft.value.plantName.trim(),
    equipmentType: draft.value.equipmentType.trim(),
    model: draft.value.model?.trim() || undefined,
    circuitPosition: draft.value.circuitPosition?.trim() || undefined,
    plantAliases: draft.value.aliasesText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
  }
  try {
    if (editingId.value) {
      await equipmentApi.update(editingId.value, body)
      toast.success('Запись обновлена')
    } else {
      await equipmentApi.create(body)
      toast.success('Оборудование добавлено')
    }
    formOpen.value = false
    await loadEquipment()
  } catch (e) {
    toast.error('Не удалось сохранить', { description: (e as Error).message })
  } finally {
    saving.value = false
  }
}

const pendingEq = ref<PlantEquipment | null>(null)
const deletingEq = ref(false)
async function confirmDeleteEq() {
  if (!pendingEq.value) return
  deletingEq.value = true
  try {
    await equipmentApi.remove(pendingEq.value.id)
    equipment.value = equipment.value.filter((x) => x.id !== pendingEq.value!.id)
    toast.success('Запись удалена')
    pendingEq.value = null
  } catch (e) {
    toast.error('Не удалось удалить', { description: (e as Error).message })
  } finally {
    deletingEq.value = false
  }
}

function fmtDate(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime())
    ? ''
    : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

// Ленивая загрузка оборудования при первом открытии вкладки.
watch(tab, (t) => {
  if (t === 'equipment' && !eqLoaded.value && !eqLoading.value) loadEquipment()
})

onMounted(loadDocs)
</script>

<template>
  <section>
    <div class="mb-6 flex items-end gap-4">
      <div>
        <h1 class="h1 mb-1 text-[26px] font-extrabold text-ink">База знаний</h1>
        <p class="text-[14px] text-sec">Входные материалы, на которых система строит гипотезы</p>
      </div>
      <div class="flex-1"></div>
      <button
        v-if="tab === 'documents'"
        class="btn btn-secondary cursor-not-allowed px-4 py-2.5 text-[13px] opacity-60"
        disabled
        title="Требуется GPU-профиль ingestion — в демо недоступно"
      >
        <Icon icon="lucide:lock" class="size-4" />
        Загрузить документ
      </button>
      <button v-else class="btn btn-primary px-4 py-2.5 text-[13px]" @click="openAdd">
        <Icon icon="lucide:plus" class="size-4" />
        Добавить оборудование
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
      <div v-if="docsLoading && !docs.length" class="flex flex-col gap-2">
        <div v-for="i in 5" :key="i" class="card h-[54px] animate-pulse bg-muted/40"></div>
      </div>
      <div v-else-if="docsError" class="card flex flex-col items-center gap-3 p-10 text-center">
        <Icon icon="lucide:cloud-alert" class="size-8 text-faint" />
        <p class="text-[14px] text-sec">Не удалось загрузить документы: {{ docsError }}</p>
        <button class="btn btn-secondary px-4 py-2 text-[13px]" @click="loadDocs">Повторить</button>
      </div>
      <div v-else-if="docsEmpty" class="card p-12 text-center text-[14px] text-sec">Корпус пуст.</div>
      <div v-else class="card overflow-hidden">
        <div class="grid grid-cols-[1fr_130px_120px_70px_110px_40px] gap-4 border-b border-bd px-5 py-3 lbl">
          <span>Название</span><span>Тип</span><span>Домен</span><span>Язык</span><span>Добавлен</span><span></span>
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
            @click="pendingDoc = doc"
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

    <!-- ОБОРУДОВАНИЕ (CRUD) -->
    <template v-else>
      <div v-if="eqLoading && !equipment.length" class="flex flex-col gap-2">
        <div v-for="i in 4" :key="i" class="card h-[54px] animate-pulse bg-muted/40"></div>
      </div>
      <div v-else-if="eqError" class="card flex flex-col items-center gap-3 p-10 text-center">
        <Icon icon="lucide:cloud-alert" class="size-8 text-faint" />
        <p class="text-[14px] text-sec">Не удалось загрузить оборудование: {{ eqError }}</p>
        <button class="btn btn-secondary px-4 py-2 text-[13px]" @click="loadEquipment">Повторить</button>
      </div>
      <div v-else-if="!equipment.length" class="card flex flex-col items-center gap-2 p-12 text-center">
        <div class="grid size-11 place-items-center rounded-full border border-bds">
          <Icon icon="lucide:settings-2" class="size-[18px] text-faint" />
        </div>
        <div class="text-[15px] font-semibold text-ink">Оборудование не добавлено</div>
        <p class="text-[13.5px] text-sec">Добавьте первую запись кнопкой сверху.</p>
      </div>
      <div v-else class="space-y-5">
        <div v-for="[plant, items] in grouped" :key="plant">
          <div class="mb-2 flex items-center gap-2">
            <Icon icon="lucide:factory" class="size-4 text-faint" />
            <span class="text-[14px] font-semibold text-ink">{{ plant }}</span>
            <span class="mono text-[11.5px] text-faint">{{ items.length }}</span>
          </div>
          <div class="card overflow-hidden">
            <div
              v-for="(e, i) in items"
              :key="e.id"
              class="row grid grid-cols-[1fr_1fr_130px_84px] items-center gap-4 px-5 py-3"
              :class="{ 'border-t border-bd': i }"
            >
              <div class="min-w-0">
                <div class="truncate text-[13.5px] font-medium text-ink">{{ e.equipmentType }}</div>
                <div v-if="e.plantAliases?.length" class="truncate text-[11px] text-faint">
                  {{ e.plantAliases.join(', ') }}
                </div>
              </div>
              <span class="truncate text-[12.5px] text-sec">{{ e.model || '—' }}</span>
              <span class="truncate text-[12px] text-faint">{{ e.circuitPosition || '—' }}</span>
              <div class="flex items-center justify-end gap-1">
                <button
                  class="grid size-8 place-items-center rounded-md text-faint transition hover:bg-muted hover:text-ink"
                  title="Изменить"
                  @click="openEdit(e)"
                >
                  <Icon icon="lucide:pencil" class="size-4" />
                </button>
                <button
                  class="grid size-8 place-items-center rounded-md text-faint transition hover:bg-muted hover:text-danger"
                  title="Удалить"
                  @click="pendingEq = e"
                >
                  <Icon icon="lucide:trash-2" class="size-4" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Форма оборудования -->
    <Teleport to="body">
      <div v-if="formOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/30" @click="formOpen = false"></div>
        <div class="card relative w-full max-w-[480px] p-6">
          <div class="mb-5 text-[16px] font-bold text-ink">
            {{ editingId ? 'Изменить оборудование' : 'Новое оборудование' }}
          </div>
          <div class="space-y-3">
            <div>
              <label class="lbl mb-1 block">Фабрика *</label>
              <input v-model="draft.plantName" class="inp" list="plant-list" placeholder="напр. КГМК" />
              <datalist id="plant-list">
                <option v-for="p in plants" :key="p.plantName" :value="p.plantName" />
              </datalist>
            </div>
            <div>
              <label class="lbl mb-1 block">Тип оборудования *</label>
              <input v-model="draft.equipmentType" class="inp" placeholder="напр. Флотомашина" />
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="lbl mb-1 block">Модель</label>
                <input v-model="draft.model" class="inp" placeholder="напр. РИФ-8.5" />
              </div>
              <div>
                <label class="lbl mb-1 block">Позиция в схеме</label>
                <input v-model="draft.circuitPosition" class="inp" placeholder="напр. основная флотация" />
              </div>
            </div>
            <div>
              <label class="lbl mb-1 block">Синонимы фабрики (через запятую)</label>
              <input v-model="draft.aliasesText" class="inp" placeholder="Кольская ГМК, КГМК" />
            </div>
          </div>
          <div class="mt-6 flex justify-end gap-2">
            <button class="btn btn-ghost px-4 py-2 text-[13px]" @click="formOpen = false">Отмена</button>
            <button class="btn btn-primary px-5 py-2 text-[13px]" :disabled="!canSave" @click="save">
              {{ saving ? 'Сохранение…' : editingId ? 'Сохранить' : 'Добавить' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Подтверждение удаления документа -->
    <Teleport to="body">
      <div v-if="pendingDoc" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/30" @click="pendingDoc = null"></div>
        <div class="card relative w-full max-w-[420px] p-6">
          <div class="mb-1 text-[16px] font-bold text-ink">Удалить документ?</div>
          <p class="mb-5 text-[13px] leading-relaxed text-sec">
            «{{ pendingDoc.title }}» будет удалён вместе со всеми фрагментами. Действие необратимо.
          </p>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost px-4 py-2 text-[13px]" @click="pendingDoc = null">Отмена</button>
            <button
              class="btn btn-primary bg-danger px-4 py-2 text-[13px] hover:bg-danger/90"
              :disabled="deletingDoc"
              @click="confirmDeleteDoc"
            >
              {{ deletingDoc ? 'Удаление…' : 'Удалить' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Подтверждение удаления оборудования -->
    <Teleport to="body">
      <div v-if="pendingEq" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/30" @click="pendingEq = null"></div>
        <div class="card relative w-full max-w-[420px] p-6">
          <div class="mb-1 text-[16px] font-bold text-ink">Удалить запись?</div>
          <p class="mb-5 text-[13px] leading-relaxed text-sec">
            «{{ pendingEq.equipmentType }}» ({{ pendingEq.plantName }}) будет удалено. Действие необратимо.
          </p>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost px-4 py-2 text-[13px]" @click="pendingEq = null">Отмена</button>
            <button
              class="btn btn-primary bg-danger px-4 py-2 text-[13px] hover:bg-danger/90"
              :disabled="deletingEq"
              @click="confirmDeleteEq"
            >
              {{ deletingEq ? 'Удаление…' : 'Удалить' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

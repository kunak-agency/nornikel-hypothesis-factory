import { http } from './client'
import type {
  CreateRunRequest,
  Run,
  RunListResponse,
  SubmitFeedbackRequest,
  FeedbackResponse,
  RunStatus,
} from './types'

const base = () => http.defaults.baseURL || ''

// Эндпоинты прогонов гипотез. POST /v1/runs асинхронный: сразу возвращает
// id+status=pending, дальше клиент поллит GET /v1/runs/{id} (лёгкого
// /status-эндпоинта в API нет).
export const runsApi = {
  // status: 'in_progress' — все, кроме done/failed (фильтр на бэке).
  list(page = 1, perPage = 20, status?: string): Promise<RunListResponse> {
    return http
      .get<RunListResponse>('/v1/runs', { params: { page, perPage, status } })
      .then((r) => r.data)
  },

  get(runId: string): Promise<Run> {
    return http.get<Run>(`/v1/runs/${runId}`).then((r) => r.data)
  },

  create(body: CreateRunRequest): Promise<Run> {
    return http.post<Run>('/v1/runs', body).then((r) => r.data)
  },

  // Прогон из Excel «хвостов»: файл + текст цели одновременно (multipart).
  createFromExcel(file: File, rawText: string): Promise<Run> {
    const form = new FormData()
    form.append('file', file)
    form.append('rawText', rawText)
    return http
      .post<Run>('/v1/runs/from-excel', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      .then((r) => r.data)
  },

  remove(runId: string): Promise<void> {
    return http.delete(`/v1/runs/${runId}`).then(() => undefined)
  },

  graph(runId: string) {
    return http.get(`/v1/runs/${runId}/graph`).then((r) => r.data)
  },

  submitFeedback(
    hypothesisId: string,
    body: SubmitFeedbackRequest,
  ): Promise<FeedbackResponse> {
    return http
      .post<FeedbackResponse>(`/v1/hypotheses/${hypothesisId}/feedback`, body)
      .then((r) => r.data)
  },

  // Ссылки на экспорт (открываются напрямую браузером).
  reportUrl(runId: string, format: 'md' | 'pdf' | 'docx' | 'csv'): string {
    return `${base()}/v1/runs/${runId}/report.${format}`
  },
  jiraUrl(runId: string, projectKey = 'RND'): string {
    return `${base()}/v1/runs/${runId}/report.jira.json?projectKey=${encodeURIComponent(projectKey)}`
  },
}

// Читаемые русские подписи статусов для бейджей/стептера.
export const STATUS_LABELS: Record<RunStatus, string> = {
  pending: 'В очереди',
  retrieving: 'Поиск в базе',
  extracting: 'Извлечение фактов',
  generating: 'Генерация',
  critiquing: 'Оценка',
  done: 'Готово',
  failed: 'Ошибка',
}

// Порядок стадий прогресса (без терминальных done/failed).
export const RUN_STAGES: RunStatus[] = [
  'pending',
  'retrieving',
  'extracting',
  'generating',
  'critiquing',
]

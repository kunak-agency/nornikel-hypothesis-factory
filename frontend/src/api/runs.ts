import { http } from './client'
import type {
  CreateRunRequest,
  Run,
  RunListResponse,
  SubmitFeedbackRequest,
  FeedbackResponse,
  RunStatus,
  RankingWeights,
  ClaimListResponse,
} from './types'

const base = () => http.defaults.baseURL || ''

// Доп. настройки для прогона из Excel (передаются в formData).
export interface ExcelRunOptions {
  language?: string
  rankingWeights?: RankingWeights
  excludedTopics?: string[]
}

// Эндпоинты прогонов гипотез. POST /v1/runs асинхронный: сразу возвращает
// id+status=pending, дальше клиент поллит GET /v1/runs/{id} (лёгкого
// /status-эндпоинта в API нет).
export const runsApi = {
  // GET /v1/runs принимает только page/perPage (фильтра по статусу в API нет —
  // фильтруем на клиенте).
  list(page = 1, perPage = 30): Promise<RunListResponse> {
    return http
      .get<RunListResponse>('/v1/runs', { params: { page, perPage } })
      .then((r) => r.data)
  },

  get(runId: string): Promise<Run> {
    return http.get<Run>(`/v1/runs/${runId}`).then((r) => r.data)
  },

  create(body: CreateRunRequest): Promise<Run> {
    return http.post<Run>('/v1/runs', body).then((r) => r.data)
  },

  // Прогон из Excel «хвостов»: файл + текст цели + опц. настройки (multipart).
  // rankingWeights/excludedTopics передаются JSON-строками (см. Swagger from-excel).
  createFromExcel(file: File, rawText: string, opts?: ExcelRunOptions): Promise<Run> {
    const form = new FormData()
    form.append('file', file)
    form.append('rawText', rawText)
    form.append('language', opts?.language ?? 'ru')
    if (opts?.rankingWeights) form.append('rankingWeights', JSON.stringify(opts.rankingWeights))
    if (opts?.excludedTopics?.length)
      form.append('excludedTopics', JSON.stringify(opts.excludedTopics))
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

  // Evidence: дословные факты-цитаты прогона.
  claims(runId: string): Promise<ClaimListResponse> {
    return http.get<ClaimListResponse>(`/v1/runs/${runId}/claims`).then((r) => r.data)
  },

  // Пересортировка ранжирования по новым весам (перезаписывает total/rank в БД).
  rerank(runId: string, rankingWeights: RankingWeights): Promise<Run> {
    return http
      .post<Run>(`/v1/runs/${runId}/rerank`, { rankingWeights })
      .then((r) => r.data)
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

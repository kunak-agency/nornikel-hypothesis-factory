import { http } from './client'
import type {
  CreateRunRequest,
  Run,
  RunListResponse,
  SubmitFeedbackRequest,
  FeedbackResponse,
} from './types'

// Эндпоинты прогонов гипотез. POST /v1/runs асинхронный: сразу возвращает
// 202 с id+status=pending, дальше клиент поллит GET /v1/runs/{id}.
export const runsApi = {
  list(page = 1, perPage = 20): Promise<RunListResponse> {
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

  reportMarkdownUrl(runId: string): string {
    return `${http.defaults.baseURL || ''}/v1/runs/${runId}/report.md`
  },

  submitFeedback(hypothesisId: string, body: SubmitFeedbackRequest): Promise<FeedbackResponse> {
    return http
      .post<FeedbackResponse>(`/v1/hypotheses/${hypothesisId}/feedback`, body)
      .then((r) => r.data)
  },
}

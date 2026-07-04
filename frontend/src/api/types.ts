// Типы ответов/запросов API. Соответствуют DTO бэкенда (in/, out/) — camelCase.

export type RunStatus =
  | 'pending'
  | 'retrieving'
  | 'extracting'
  | 'generating'
  | 'critiquing'
  | 'done'
  | 'failed'

export interface ProblemSpec {
  targetKpi: string
  plant: string
  targetMetals: string[]
  lossHotspots: string[]
  availableEquipment: string[]
  constraints: string[]
  horizon: string
}

export interface KPIEffect {
  metric: string
  direction: 'increase' | 'decrease' | string
  magnitude: string
}

export interface Scores {
  evidenceStrength: number
  feasibility: number
  impact: number
  novelty: number
  riskPenalty: number
  confidence: number
  total: number
}

export interface VerificationStep {
  step: string
  resource: string
  successCriterion: string
  estimatedDuration?: string
  estimatedCost?: string
}

export interface Hypothesis {
  id: string
  runId: string
  statement: string
  mechanism: string
  evidenceRefs: string[]
  expectedKpiEffect: KPIEffect
  risks: string[]
  noveltyReason: string
  verificationPlan: VerificationStep[]
  scores: Scores
  criticNotes: string
  rank: number
  createdAt: string
}

export interface Run {
  id: string
  status: RunStatus
  problemSpec: ProblemSpec
  domain: string
  language: string
  knowledgeGaps?: string[]
  error?: string
  createdAt: string
  completedAt?: string
  hypotheses?: Hypothesis[]
}

export interface RunListResponse {
  items: Run[]
  total: number
  page: number
  perPage: number
}

export interface RankingWeights {
  evidenceStrength?: number
  feasibility?: number
  impact?: number
  novelty?: number
  riskPenalty?: number
}

export interface CreateRunRequest {
  rawText: string
  rawInput?: Record<string, unknown>
  domain?: string
  language?: 'ru' | 'en' | 'zh'
  rankingWeights?: RankingWeights
  excludedTopics?: string[]
}

export interface DocumentItem {
  id: string
  title: string
  sourceType: string
  domain: string
  language: string
  chunkCount: number
  createdAt: string
}

export interface DocumentListResponse {
  items: DocumentItem[]
  total: number
}

export type FeedbackVerdict = 'confirmed' | 'rejected' | 'needs_revision'

export interface SubmitFeedbackRequest {
  verdict: FeedbackVerdict
  comment?: string
  reviewer?: string
}

export interface FeedbackResponse {
  id: string
  hypothesisId: string
  verdict: string
  comment: string
  reviewer: string
  createdAt: string
}

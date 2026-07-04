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

// Соответствует domain.RankingWeights в Swagger: поля evidence/risk
// (НЕ evidenceStrength/riskPenalty — иначе бэкенд молча игнорирует эти веса).
export interface RankingWeights {
  evidence?: number
  feasibility?: number
  impact?: number
  novelty?: number
  risk?: number
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

// --- Репутация сущностей (GET /v1/entities/reputation) ---
export interface EntityReputation {
  entityId: string
  canonicalName: string
  confirmed: number
  rejected: number
  needsRevision: number
}
export interface EntityReputationListResponse {
  items: EntityReputation[]
}

// --- Оборудование фабрик (/v1/plant-equipment) ---
export interface PlantEquipment {
  id: string
  plantName: string
  equipmentType: string
  model: string
  circuitPosition: string
  plantAliases: string[]
  parameters: Record<string, unknown>
  createdAt: string
}
export interface PlantEquipmentRequest {
  plantName: string
  equipmentType: string
  model?: string
  circuitPosition?: string
  plantAliases?: string[]
  parameters?: Record<string, unknown>
}
export interface PlantEquipmentListResponse {
  items: PlantEquipment[]
  total: number
}
export interface PlantSummary {
  plantName: string
  equipmentCount: number
}
export interface PlantsResponse {
  items: PlantSummary[]
}

// --- Claims / Evidence (GET /v1/runs/:id/claims) ---
export interface Claim {
  id: string
  subject: string
  action: string
  condition: string
  metric: string
  effectDirection: string
  effectMagnitude: string
  quote: string
  source: Record<string, unknown>
  sourceConfidence: string
  citedByHypothesisIds: string[]
  createdAt: string
}
export interface ClaimListResponse {
  items: Claim[]
  total: number
}

// --- Rerank (POST /v1/runs/:id/rerank) ---
export interface RerankRequest {
  rankingWeights: RankingWeights
}

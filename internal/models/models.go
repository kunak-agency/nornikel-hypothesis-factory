package models

import (
	"time"

	"github.com/google/uuid"
)

type Document struct {
	ID         uuid.UUID
	Title      string
	SourceType string // book | regulation | scheme | historical_case | report
	FilePath   string
	Domain     string
	Language   string
	Metadata   map[string]any
	CreatedAt  time.Time
}

type Chunk struct {
	ID          uuid.UUID
	DocumentID  uuid.UUID
	Ordinal     int
	Section     string
	Content     string
	ContentType string // text | table | figure_caption
	Embedding   []float32
	Metadata    map[string]any
	CreatedAt   time.Time
}

// RetrievedChunk carries fused hybrid-retrieval scores alongside the chunk.
type RetrievedChunk struct {
	Chunk
	LexicalScore float64
	VectorScore  float64
	FusedScore   float64
	DocumentTitle string
	SourceType    string
}

type Claim struct {
	ID                uuid.UUID
	ChunkID           uuid.UUID
	Subject           string
	Action            string
	Condition         string
	Metric            string
	EffectDirection   string // increase | decrease | neutral | mixed
	EffectMagnitude   string
	SourceConfidence  string // low | medium | high
	ConflictFlag      bool
	Quote             string
	Metadata          map[string]any
}

// ProblemSpec is the structured capture of a user request: target KPI,
// object of intervention, constraints. Drives retrieval filters and
// keeps hypothesis generation grounded in what's actually achievable.
type ProblemSpec struct {
	TargetKPI          string   `json:"target_kpi"`           // e.g. "снижение потерь Ni в отвальных хвостах"
	Plant              string   `json:"plant"`                // e.g. "КГМК"
	TargetMetals       []string `json:"target_metals"`        // ["Ni", "Cu"]
	LossHotspots       []string `json:"loss_hotspots"`        // e.g. ["класс -71+45 мкм, закрытый Pnt/Cp"]
	AvailableEquipment []string `json:"available_equipment"`
	Constraints        []string `json:"constraints"`          // budget, normative, forbidden directions
	Horizon            string   `json:"horizon"`
}

type HypothesisRun struct {
	ID          uuid.UUID
	ProblemSpec ProblemSpec
	RawInput    map[string]any
	Status      string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

type KPIEffect struct {
	Metric    string `json:"metric"`
	Direction string `json:"direction"` // increase | decrease
	Magnitude string `json:"magnitude"` // qualitative or quantitative estimate
}

type Scores struct {
	EvidenceStrength float64 `json:"evidence_strength"`
	Feasibility      float64 `json:"feasibility"`
	Impact           float64 `json:"impact"`
	Novelty          float64 `json:"novelty"`
	RiskPenalty      float64 `json:"risk_penalty"`
	Confidence       float64 `json:"confidence"`
	Total            float64 `json:"total"`
}

type VerificationStep struct {
	Step        string `json:"step"`
	Resource    string `json:"resource"`
	SuccessCrit string `json:"success_criterion"`
}

type Hypothesis struct {
	ID                uuid.UUID
	RunID             uuid.UUID
	Statement         string
	Mechanism         string
	EvidenceRefs      []uuid.UUID
	ExpectedKPIEffect KPIEffect
	Risks             []string
	NoveltyReason     string
	VerificationPlan  []VerificationStep
	Scores            Scores
	CriticNotes       string
	Rank              int
	CreatedAt         time.Time
}

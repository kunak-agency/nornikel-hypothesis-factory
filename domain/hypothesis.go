package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KPIEffect/Scores/VerificationStep — простые value-объекты без собственного
// поведения. В отличие от ProblemSpec их JSON-теги не завязаны на контракт с
// LLM (services/hypothesisfactory парсит вывод модели в отдельные анонимные
// структуры и копирует поля сюда вручную), поэтому теги — сразу camelCase и
// переиспользуются как есть в out.HypothesisResponse без отдельного маппинга.
type KPIEffect struct {
	Metric    string `json:"metric"`
	Direction string `json:"direction"` // increase | decrease
	Magnitude string `json:"magnitude"`
}

type Scores struct {
	EvidenceStrength float64 `json:"evidenceStrength"`
	Feasibility      float64 `json:"feasibility"`
	Impact           float64 `json:"impact"`
	Novelty          float64 `json:"novelty"`
	RiskPenalty      float64 `json:"riskPenalty"`
	Confidence       float64 `json:"confidence"`
	Total            float64 `json:"total"`
}

// VerificationStep — один шаг дорожной карты проверки гипотезы.
// EstimatedDuration/EstimatedCost — качественная/количественная оценка
// ("1-2 недели", "~200 т.р. на реагенты"), не строгое число: LLM оценивает
// по контексту, не считает по формуле — но этого достаточно, чтобы
// фронтенд построил визуальный таймлайн/бюджет без выдумывания полей.
type VerificationStep struct {
	Step              string `json:"step"`
	Resource          string `json:"resource"`
	SuccessCrit       string `json:"successCriterion"`
	EstimatedDuration string `json:"estimatedDuration,omitempty"`
	EstimatedCost     string `json:"estimatedCost,omitempty"`
}

// Hypothesis — одна сгенерированная гипотеза, привязанная к конкретным
// Claim.ID через EvidenceRefs (интерпретируемость: источник → claim →
// гипотеза видна целиком). Rank проставляется детерминированно после
// критик-ансамбля (services/hypothesisfactory), не LLM.
type Hypothesis struct {
	ID                uuid.UUID          `gorm:"type:uuid;primaryKey"`
	RunID             uuid.UUID          `gorm:"type:uuid;not null;index"`
	Statement         string             `gorm:"type:text;not null"`
	Mechanism         string             `gorm:"type:text;not null"`
	EvidenceRefs      []uuid.UUID        `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	ExpectedKPIEffect KPIEffect          `gorm:"type:jsonb;serializer:json;not null"`
	Risks             []string           `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	NoveltyReason     string             `gorm:"type:text;not null;default:''"`
	VerificationPlan  []VerificationStep `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	Scores            Scores             `gorm:"type:jsonb;serializer:json;not null"`
	CriticNotes       string             `gorm:"type:text;not null;default:''"`
	Rank              int                `gorm:"not null;default:0"`
	CreatedAt         time.Time
}

func (h *Hypothesis) BeforeCreate(tx *gorm.DB) error {
	return NewIDIfEmpty(&h.ID)
}

package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProblemSpec — структурированный запрос: цель/KPI, объект воздействия,
// ограничения. Теги — snake_case НЕ ради API-контракта, а потому что это
// ровно та JSON-схема, которую пишет в ответе LLM (services/hypothesisfactory
// парсит вывод модели прямо в этот тип); поле не тронуто, если поменять теги —
// парсинг ответа LLM сломается. Для API-ответов есть out.ProblemSpecResponse
// с camelCase.
type ProblemSpec struct {
	TargetKPI          string   `json:"target_kpi"`
	Plant              string   `json:"plant"`
	TargetMetals       []string `json:"target_metals"`
	LossHotspots       []string `json:"loss_hotspots"`
	AvailableEquipment []string `json:"available_equipment"`
	Constraints        []string `json:"constraints"`
	Horizon            string   `json:"horizon"`
}

// Статусы HypothesisRun — прогресс пайплайна, по которому фронт поллит
// GET /runs/:id (POST /runs возвращает управление сразу после ProblemSpec,
// остальные стадии выполняются в фоне).
const (
	RunStatusPending     = "pending"
	RunStatusRetrieving  = "retrieving"
	RunStatusExtracting  = "extracting"
	RunStatusGenerating  = "generating"
	RunStatusCritiquing  = "critiquing"
	RunStatusDone        = "done"
	RunStatusFailed      = "failed"
)

type HypothesisRun struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProblemSpec ProblemSpec    `gorm:"type:jsonb;serializer:json;not null"`
	RawInput    map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	Status      string         `gorm:"type:text;not null;default:'pending';index"`
	Error       string         `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time
	CompletedAt *time.Time
}

func (r *HypothesisRun) BeforeCreate(tx *gorm.DB) error {
	if r.ID != uuid.Nil {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	r.ID = id
	return nil
}

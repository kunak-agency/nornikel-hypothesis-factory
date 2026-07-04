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

// RankingWeights — режим "экспертной настройки" ранжирования (кейс явно
// просит: "возможность задавать веса критериев ранжирования"). nil-поле =
// дефолтный вес (см. hypothesisfactory.defaultRankingWeights); заполняются
// только те критерии, которые эксперт хочет переопределить.
type RankingWeights struct {
	Evidence    *float64 `json:"evidence,omitempty"`
	Feasibility *float64 `json:"feasibility,omitempty"`
	Impact      *float64 `json:"impact,omitempty"`
	Novelty     *float64 `json:"novelty,omitempty"`
	Risk        *float64 `json:"risk,omitempty"`
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
	// Domain — фильтр базы знаний (documents.domain) для retrieval; параметр
	// запроса, а не константа сервиса — то самое "подключение новых
	// предметных областей без перестройки ядра" из требований кейса.
	Domain string `gorm:"type:text;not null;default:'flotation'"`
	// Language — язык вывода гипотез (ru|en|zh), не язык исходников (те
	// смешанные по языку) — требование мультиязычности кейса.
	Language string `gorm:"type:text;not null;default:'ru'"`
	// RankingWeights/ExcludedTopics — "режим экспертной настройки" из
	// доп. пожеланий кейса: веса критериев ранжирования и исключаемые
	// направления, на уровне конкретного прогона.
	RankingWeights RankingWeights `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	ExcludedTopics []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	// KnowledgeGaps — детерминированно найденные точки ProblemSpec (металлы/
	// точки потерь), слабо покрытые retrieved evidence — "выявление пробелов
	// в знаниях" из функциональных требований кейса.
	KnowledgeGaps []string `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	Status        string   `gorm:"type:text;not null;default:'pending';index"`
	Error          string         `gorm:"type:text;not null;default:''"`
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

func (r *HypothesisRun) BeforeCreate(tx *gorm.DB) error {
	return NewIDIfEmpty(&r.ID)
}

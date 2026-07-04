package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Feedback — экспертная оценка одной гипотезы (confirmed/rejected/needs_revision).
// "Обучение на фидбэке" из кейса реализовано как граф репутации сущностей
// (см. services/hypothesisfactory/(*Service).loadEntityReputations,
// repositories/entity.go's GetFeedbackStats) — join claims→hypotheses.
// evidence_refs→feedbacks через entity resolution, а не плоская инъекция
// фидбэка в промпт.
type Feedback struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	HypothesisID uuid.UUID `gorm:"type:uuid;not null;index"`
	Verdict      string    `gorm:"type:text;not null"` // confirmed | rejected | needs_revision
	Comment      string    `gorm:"type:text;not null;default:''"`
	Reviewer     string    `gorm:"type:text;not null;default:''"`
	CreatedAt    time.Time
}

func (f *Feedback) BeforeCreate(tx *gorm.DB) error {
	return NewIDIfEmpty(&f.ID)
}

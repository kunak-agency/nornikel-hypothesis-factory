package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Feedback — экспертная оценка одной гипотезы (confirmed/rejected/needs_revision).
// Задел под "обучение на фидбэке" из кейса: сейчас только пишется, в
// ранжировании/генерации не используется.
type Feedback struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	HypothesisID uuid.UUID `gorm:"type:uuid;not null;index"`
	Verdict      string    `gorm:"type:text;not null"` // confirmed | rejected | needs_revision
	Comment      string    `gorm:"type:text;not null;default:''"`
	Reviewer     string    `gorm:"type:text;not null;default:''"`
	CreatedAt    time.Time
}

func (f *Feedback) BeforeCreate(tx *gorm.DB) error {
	if f.ID != uuid.Nil {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

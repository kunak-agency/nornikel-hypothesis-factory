package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Claim — извлечённый из чанка проверяемый факт с дословной цитатой-основанием.
// Grounding (что Quote реально встречается в исходном чанке) проверяется
// детерминированно в services/hypothesisfactory, не здесь — Claim хранит
// только уже провалидированные факты.
type Claim struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ChunkID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	Subject          string         `gorm:"type:text;not null"`
	SubjectEntityID  *uuid.UUID     `gorm:"type:uuid;index"` // резолвится через embedding similarity, см. services/hypothesisfactory/entities.go
	Action           string         `gorm:"type:text;not null"`
	Condition        string         `gorm:"type:text;not null;default:''"`
	Metric           string         `gorm:"type:text;not null;default:''"`
	MetricEntityID   *uuid.UUID     `gorm:"type:uuid;index"`
	EffectDirection  string         `gorm:"type:text;not null;default:''"` // increase | decrease | neutral | mixed | unspecified
	EffectMagnitude  string         `gorm:"type:text;not null;default:''"`
	SourceConfidence string         `gorm:"type:text;not null;default:'medium'"` // low | medium | high
	ConflictFlag     bool           `gorm:"not null;default:false"`
	Quote            string         `gorm:"type:text;not null"`
	Metadata         map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	CreatedAt        time.Time

	// SubjectKind/MetricKind — транзиентная разметка от LLM (equipment/metric/
	// reagent/...), не персистится (kind живёт на Entity, не на Claim) — нужна
	// только чтобы донести её от extraction до entity resolution в одном прогоне.
	SubjectKind string `gorm:"-"`
	MetricKind  string `gorm:"-"`
}

func (c *Claim) BeforeCreate(tx *gorm.DB) error {
	if c.ID != uuid.Nil {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

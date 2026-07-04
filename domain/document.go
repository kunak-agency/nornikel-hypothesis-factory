package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Document — единица базы знаний (учебник, регламент, схема, исторический
// пример). Chunks режутся из него на этапе ingestion и хранят FK на него.
type Document struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Title      string         `gorm:"type:text;not null"`
	SourceType string         `gorm:"type:text;not null;index"` // book | regulation | scheme | historical_case | report
	FilePath   string         `gorm:"type:text;not null;default:''"`
	Domain     string         `gorm:"type:text;not null;default:'flotation';index"`
	Language   string         `gorm:"type:text;not null;default:'ru'"`
	Metadata   map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	CreatedAt  time.Time
}

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	return NewIDIfEmpty(&d.ID)
}

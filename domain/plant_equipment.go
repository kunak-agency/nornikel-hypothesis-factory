package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PlantEquipment — структурированная запись оборудования конкретной фабрики,
// извлечённая из регламентов/схем. В отличие от того же факта, зарытого в
// прозе markdown-чанка, эта запись находится ДЕТЕРМИНИРОВАННО (SQL по имени
// фабрики), а не через RAG-поиск, которому нужно "повезти" с фразировкой
// запроса — источник точных гипотез в духе "диаметр насадки X→Y", а не общих
// формулировок "оптимизировать классификацию".
type PlantEquipment struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	PlantName    string    `gorm:"type:text;not null;index"` // каноническое имя, напр. "ТОФ"
	PlantAliases []string  `gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	// EquipmentType: hydrocyclone | mill | classifier | flotation_cell |
	// crusher | screen | thickener | pump
	EquipmentType    string         `gorm:"type:text;not null"`
	Model            string         `gorm:"type:text;not null;default:''"`
	Parameters       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	CircuitPosition  string         `gorm:"type:text;not null;default:''"`
	SourceDocumentID *uuid.UUID     `gorm:"type:uuid"`
	CreatedAt        time.Time
}

func (e *PlantEquipment) BeforeCreate(tx *gorm.DB) error {
	return NewIDIfEmpty(&e.ID)
}

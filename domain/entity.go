package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

// Entity — узел графа знаний (оборудование/показатель/реагент/процесс/материал),
// резолвится через embedding similarity при claim extraction: похожая по смыслу
// сущность из разных источников/прогонов схлопывается в одну (аналог
// "склеивания похожих сущностей" в memory-архитектурах), а не дублируется на
// каждый чанк заново. Это то, что делает граф накопленной структурой памяти
// между прогонами, а не одноразовой вьюхой поверх текста одного прогона.
const (
	EntityKindEquipment = "equipment"
	EntityKindMetric    = "metric"
	EntityKindReagent   = "reagent"
	EntityKindProcess   = "process"
	EntityKindMaterial  = "material"
	EntityKindOther     = "other"
)

type Entity struct {
	ID            uuid.UUID        `gorm:"type:uuid;primaryKey"`
	CanonicalName string           `gorm:"type:text;not null"`
	Kind          string           `gorm:"type:text;not null;default:'other';index"`
	Embedding     *pgvector.Vector `gorm:"type:vector(1024)"`
	CreatedAt     time.Time
}

func (e *Entity) BeforeCreate(tx *gorm.DB) error {
	if e.ID != uuid.Nil {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	e.ID = id
	return nil
}

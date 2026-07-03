package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

// Chunk — семантический фрагмент документа (секция/таблица), полученный
// Docling-чанкером. Embedding — dense-вектор BGE-M3 (1024 измерения);
// tsvector-колонка для лексического поиска генерируется в БД (см. миграцию),
// в структуре не представлена — гибридный поиск ходит туда raw SQL-запросом.
type Chunk struct {
	ID          uuid.UUID        `gorm:"type:uuid;primaryKey"`
	DocumentID  uuid.UUID        `gorm:"type:uuid;not null;index"`
	Ordinal     int              `gorm:"not null;default:0"`
	Section     string           `gorm:"type:text;not null;default:''"`
	Content     string           `gorm:"type:text;not null"`
	ContentType string           `gorm:"type:text;not null;default:'text'"` // text | table | figure_caption
	Embedding   *pgvector.Vector `gorm:"type:vector(1024)"`
	Metadata    map[string]any   `gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	CreatedAt   time.Time
}

func (c *Chunk) BeforeCreate(tx *gorm.DB) error {
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

// RetrievedChunk несёт чанк вместе со скорингом гибридного поиска и метаданными
// родительского документа — то, что реально нужно downstream claim extraction.
type RetrievedChunk struct {
	Chunk
	LexicalScore  float64
	VectorScore   float64
	FusedScore    float64
	DocumentTitle string
	SourceType    string
}

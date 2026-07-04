package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DocumentRepo struct{ db *gorm.DB }

func NewDocumentRepo(db *gorm.DB) *DocumentRepo { return &DocumentRepo{db: db} }

func (r *DocumentRepo) Create(ctx context.Context, d *domain.Document) error {
	return r.db.WithContext(ctx).Create(d).Error
}

// DocumentWithChunkCount — документ + число нарезанных из него chunks,
// нужно для списка базы знаний в UI (показать, сколько всего проиндексировано).
type DocumentWithChunkCount struct {
	domain.Document
	ChunkCount int64
}

func (r *DocumentRepo) List(ctx context.Context) ([]DocumentWithChunkCount, error) {
	var out []DocumentWithChunkCount
	err := r.db.WithContext(ctx).
		Table("documents AS d").
		Select("d.*, COUNT(c.id) AS chunk_count").
		Joins("LEFT JOIN chunks c ON c.document_id = d.id").
		Group("d.id").
		Order("d.created_at DESC").
		Scan(&out).Error
	return out, err
}

func (r *DocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	var d domain.Document
	err := r.db.WithContext(ctx).First(&d, "id = ?", id).Error
	return ignoreNotFound(&d, err)
}

// Delete каскадно удаляет chunks/claims через FK ON DELETE CASCADE (см.
// Repos.MigrateDB). Возвращает число удалённых строк документа (0 = не найден).
func (r *DocumentRepo) Delete(ctx context.Context, id uuid.UUID) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&domain.Document{}, "id = ?", id)
	return res.RowsAffected, res.Error
}

func ignoreNotFound[T any](v *T, err error) (*T, error) {
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

// GetWithChunkCount — один документ с числом его chunks (деталь для UI).
func (r *DocumentRepo) GetWithChunkCount(ctx context.Context, id uuid.UUID) (*DocumentWithChunkCount, error) {
	var out DocumentWithChunkCount
	err := r.db.WithContext(ctx).
		Table("documents AS d").
		Select("d.*, COUNT(c.id) AS chunk_count").
		Joins("LEFT JOIN chunks c ON c.document_id = d.id").
		Where("d.id = ?", id).
		Group("d.id").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	if out.ID == uuid.Nil {
		return nil, nil
	}
	return &out, nil
}

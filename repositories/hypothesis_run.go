package repositories

import (
	"context"
	"encoding/json"
	"time"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HypothesisRunRepo struct{ db *gorm.DB }

func NewHypothesisRunRepo(db *gorm.DB) *HypothesisRunRepo { return &HypothesisRunRepo{db: db} }

func (r *HypothesisRunRepo) Create(ctx context.Context, run *domain.HypothesisRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *HypothesisRunRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.HypothesisRun, error) {
	var run domain.HypothesisRun
	err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error
	return ignoreNotFound(&run, err)
}

// List возвращает страницу прогонов, самые новые первыми — история для UI.
func (r *HypothesisRunRepo) List(ctx context.Context, offset, limit int) ([]domain.HypothesisRun, int64, error) {
	var out []domain.HypothesisRun
	var total int64
	if err := r.db.WithContext(ctx).Model(&domain.HypothesisRun{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.db.WithContext(ctx).Order("created_at DESC").Offset(offset).Limit(limit).Find(&out).Error
	return out, total, err
}

func (r *HypothesisRunRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.db.WithContext(ctx).Model(&domain.HypothesisRun{}).Where("id = ?", id).
		Update("status", status).Error
}

func (r *HypothesisRunRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	return r.db.WithContext(ctx).Model(&domain.HypothesisRun{}).Where("id = ?", id).
		Updates(map[string]any{"status": domain.RunStatusFailed, "error": reason}).Error
}

func (r *HypothesisRunRepo) MarkDone(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&domain.HypothesisRun{}).Where("id = ?", id).
		Updates(map[string]any{"status": domain.RunStatusDone, "completed_at": now}).Error
}

func (r *HypothesisRunRepo) UpdateKnowledgeGaps(ctx context.Context, id uuid.UUID, gaps []string) error {
	if gaps == nil {
		gaps = []string{}
	}
	// Update() с column name + raw Go value обходит GORM-сериализатор
	// (serializer:json на поле модели) — тот применяется только когда апдейт
	// идёт через сам struct-field. Раз мы бьём по колонке напрямую,
	// сериализуем в JSON сами, иначе Postgres видит Go-слайс как composite
	// (record) литерал и ругается на несовпадение типов с jsonb.
	raw, err := json.Marshal(gaps)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&domain.HypothesisRun{}).Where("id = ?", id).
		Update("knowledge_gaps", raw).Error
}

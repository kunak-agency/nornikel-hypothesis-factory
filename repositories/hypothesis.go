package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HypothesisRepo struct{ db *gorm.DB }

func NewHypothesisRepo(db *gorm.DB) *HypothesisRepo { return &HypothesisRepo{db: db} }

func (r *HypothesisRepo) Create(ctx context.Context, h *domain.Hypothesis) error {
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *HypothesisRepo) GetByRunID(ctx context.Context, runID uuid.UUID) ([]domain.Hypothesis, error) {
	var out []domain.Hypothesis
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("rank ASC").Find(&out).Error
	return out, err
}

func (r *HypothesisRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Hypothesis, error) {
	var h domain.Hypothesis
	err := r.db.WithContext(ctx).First(&h, "id = ?", id).Error
	return ignoreNotFound(&h, err)
}

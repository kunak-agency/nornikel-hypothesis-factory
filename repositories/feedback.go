package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FeedbackRepo struct{ db *gorm.DB }

func NewFeedbackRepo(db *gorm.DB) *FeedbackRepo { return &FeedbackRepo{db: db} }

func (r *FeedbackRepo) Create(ctx context.Context, f *domain.Feedback) error {
	return r.db.WithContext(ctx).Create(f).Error
}

// ListByHypothesis — все экспертные оценки одной гипотезы, новые первыми.
func (r *FeedbackRepo) ListByHypothesis(ctx context.Context, hypothesisID uuid.UUID) ([]domain.Feedback, error) {
	var out []domain.Feedback
	err := r.db.WithContext(ctx).Where("hypothesis_id = ?", hypothesisID).Order("created_at DESC").Find(&out).Error
	return out, err
}

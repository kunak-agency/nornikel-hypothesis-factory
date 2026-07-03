package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"gorm.io/gorm"
)

type FeedbackRepo struct{ db *gorm.DB }

func NewFeedbackRepo(db *gorm.DB) *FeedbackRepo { return &FeedbackRepo{db: db} }

func (r *FeedbackRepo) Create(ctx context.Context, f *domain.Feedback) error {
	return r.db.WithContext(ctx).Create(f).Error
}

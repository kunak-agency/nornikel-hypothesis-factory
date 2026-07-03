package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ClaimRepo struct{ db *gorm.DB }

func NewClaimRepo(db *gorm.DB) *ClaimRepo { return &ClaimRepo{db: db} }

func (r *ClaimRepo) Create(ctx context.Context, c *domain.Claim) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *ClaimRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Claim, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []domain.Claim
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&out).Error
	return out, err
}

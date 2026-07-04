package repositories

import (
	"context"
	"encoding/json"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HypothesisRepo struct{ db *gorm.DB }

func NewHypothesisRepo(db *gorm.DB) *HypothesisRepo { return &HypothesisRepo{db: db} }

func (r *HypothesisRepo) Create(ctx context.Context, h *domain.Hypothesis) error {
	return r.db.WithContext(ctx).Create(h).Error
}

// CreateBatch вставляет все гипотезы прогона одним batched INSERT вместо
// одного round-trip'а на гипотезу (обычно 5-10 за прогон).
func (r *HypothesisRepo) CreateBatch(ctx context.Context, hyps []domain.Hypothesis) error {
	if len(hyps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(hyps, 50).Error
}

func (r *HypothesisRepo) GetByRunID(ctx context.Context, runID uuid.UUID) ([]domain.Hypothesis, error) {
	var out []domain.Hypothesis
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("rank ASC").Find(&out).Error
	return out, err
}

func (r *HypothesisRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Hypothesis, error) {
	var h domain.Hypothesis
	err := r.db.WithContext(ctx).First(&h, "id = ?", id).Error
	return requireFound(&h, err, "hypothesis")
}

// UpdateScoresAndRank сохраняет пересчитанные Total/Rank после пересортировки
// с новыми весами (POST /runs/{id}/rerank) — компоненты оценок судей не
// меняются, только итог и позиция.
func (r *HypothesisRepo) UpdateScoresAndRank(ctx context.Context, h *domain.Hypothesis) error {
	scores, err := json.Marshal(h.Scores)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&domain.Hypothesis{}).Where("id = ?", h.ID).
		Updates(map[string]any{"scores": scores, "rank": h.Rank}).Error
}

// UpdateVerificationPlan сохраняет отредактированную пользователем дорожную карту
// проверки (визуальный конструктор в UI, PUT /hypotheses/{id}/verification-plan).
// Частичный апдейт одной колонки — как UpdateScoresAndRank, — чтобы не конфликтовать
// с параллельным rerank (тот пишет scores/rank). Сериализуем в JSON сами: Update по
// имени колонки обходит GORM-сериализатор (см. UpdateKnowledgeGaps).
func (r *HypothesisRepo) UpdateVerificationPlan(ctx context.Context, h *domain.Hypothesis) error {
	plan, err := json.Marshal(h.VerificationPlan)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&domain.Hypothesis{}).Where("id = ?", h.ID).
		Update("verification_plan", plan).Error
}

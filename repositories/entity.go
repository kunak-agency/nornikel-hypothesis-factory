package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

type EntityRepo struct{ db *gorm.DB }

func NewEntityRepo(db *gorm.DB) *EntityRepo { return &EntityRepo{db: db} }

func (r *EntityRepo) Create(ctx context.Context, e *domain.Entity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// FindNearest возвращает ближайшую по cosine distance сущность заданного
// kind и её distance (0 = идентичны, 2 = противоположны). Вызывающий сам
// решает порог "это та же сущность" — см. resolveEntity в
// services/hypothesisfactory. kind сужает поиск: "диаметр насадки" (metric) и
// "гидроциклон" (equipment) не должны схлопнуться даже при похожем эмбеддинге.
func (r *EntityRepo) FindNearest(ctx context.Context, embedding []float32, kind string) (*domain.Entity, float64, error) {
	var row struct {
		domain.Entity
		Distance float64
	}
	err := r.db.WithContext(ctx).
		Table("entities").
		Select("*, embedding <=> ? AS distance", pgvector.NewVector(embedding)).
		Where("kind = ? AND embedding IS NOT NULL", kind).
		Order("distance ASC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, 0, err
	}
	if row.ID == uuid.Nil {
		return nil, 0, nil
	}
	return &row.Entity, row.Distance, nil
}

func (r *EntityRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Entity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []domain.Entity
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&out).Error
	return out, err
}

// FeedbackStats — сколько раз claims об этой сущности цитировались
// гипотезами с тем или иным экспертным вердиктом, по всем прошлым прогонам.
type FeedbackStats struct {
	EntityID      uuid.UUID
	CanonicalName string
	Confirmed     int
	Rejected      int
	NeedsRevision int
}

// GetFeedbackStats — "обучение на фидбэке" через граф памяти: entity уже
// резолвится между прогонами (embedding similarity dedup в
// services/hypothesisfactory/entities.go), так что достаточно поднять
// историю подтверждений/отклонений по entity_id через claims->hypotheses->
// feedback. Джойнит entities здесь же за CanonicalName, чтобы не делать
// отдельный GetByIDs вторым round-trip'ом на то же множество ID.
func (r *EntityRepo) GetFeedbackStats(ctx context.Context, entityIDs []uuid.UUID) ([]FeedbackStats, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	// IN ?, не = ANY(?): GORM разворачивает []uuid.UUID в перечисление
	// значений через запятую, что синтаксически валидно для IN, но не для
	// ANY (тому нужен массив) — с ANY запрос падал с syntax error.
	var out []FeedbackStats
	err := r.db.WithContext(ctx).Raw(`
		WITH claim_entities AS (
			SELECT id AS claim_id, subject_entity_id AS entity_id FROM claims WHERE subject_entity_id IN ?
			UNION ALL
			SELECT id AS claim_id, metric_entity_id AS entity_id FROM claims WHERE metric_entity_id IN ?
		)
		SELECT ce.entity_id, e.canonical_name,
		       count(*) FILTER (WHERE f.verdict = 'confirmed') AS confirmed,
		       count(*) FILTER (WHERE f.verdict = 'rejected') AS rejected,
		       count(*) FILTER (WHERE f.verdict = 'needs_revision') AS needs_revision
		FROM claim_entities ce
		JOIN entities e ON e.id = ce.entity_id
		JOIN hypotheses h ON h.evidence_refs @> jsonb_build_array(ce.claim_id::text)
		JOIN feedbacks f ON f.hypothesis_id = h.id
		GROUP BY ce.entity_id, e.canonical_name
	`, entityIDs, entityIDs).Scan(&out).Error
	return out, err
}
